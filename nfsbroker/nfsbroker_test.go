package nfsbroker_test

import (
	"bytes"
	"errors"

	"code.cloudfoundry.org/lager/lagertest"
	"github.com/pivotal-cf/brokerapi"

	"context"

	"encoding/json"

	"fmt"

	"code.cloudfoundry.org/goshims/osshim/os_fake"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/nfsbroker/nfsbroker"
	"code.cloudfoundry.org/nfsbroker/nfsbrokerfakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Broker", func() {
	var (
		broker    *nfsbroker.Broker
		fakeOs    *os_fake.FakeOs
		logger    lager.Logger
		ctx       context.Context
		fakeStore *nfsbrokerfakes.FakeStore
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test-broker")
		ctx = context.TODO()
		fakeOs = &os_fake.FakeOs{}
		fakeStore = &nfsbrokerfakes.FakeStore{}
	})

	Context("when creating first time", func() {
		BeforeEach(func() {
			broker = nfsbroker.New(
				logger,
				"service-name", "service-id", "/fake-dir",
				fakeOs,
				nil,
				fakeStore,
			)
		})

		Context(".Services", func() {
			It("returns the service catalog as appropriate", func() {
				result := broker.Services(ctx)[0]
				Expect(result.ID).To(Equal("service-id"))
				Expect(result.Name).To(Equal("service-name"))
				Expect(result.Description).To(Equal("Existing NFSv3 volumes (see: https://code.cloudfoundry.org/nfs-volume-release/)"))
				Expect(result.Bindable).To(Equal(true))
				Expect(result.PlanUpdatable).To(Equal(false))
				Expect(result.Tags).To(ContainElement("nfs"))
				Expect(result.Requires).To(ContainElement(brokerapi.RequiredPermission("volume_mount")))

				Expect(result.Plans[0].Name).To(Equal("Existing"))
				Expect(result.Plans[0].ID).To(Equal("Existing"))
				Expect(result.Plans[0].Description).To(Equal("A preexisting filesystem"))
			})
		})

		Context(".Provision", func() {
			var (
				instanceID       string
				provisionDetails brokerapi.ProvisionDetails
				asyncAllowed     bool

				spec brokerapi.ProvisionedServiceSpec
				err  error
			)

			BeforeEach(func() {
				instanceID = "some-instance-id"

				configuration := map[string]interface{}{"share": "server:/some-share"}
				buf := &bytes.Buffer{}
				_ = json.NewEncoder(buf).Encode(configuration)
				provisionDetails = brokerapi.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}
				asyncAllowed = false
			})

			JustBeforeEach(func() {
				spec, err = broker.Provision(ctx, instanceID, provisionDetails, asyncAllowed)
			})

			It("should not error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("should provision the service instance synchronously", func() {
				Expect(spec.IsAsync).To(Equal(false))
			})

			It("should write state", func() {
				_, data, id, _ := fakeStore.SaveArgsForCall(fakeStore.SaveCallCount() - 1)
				Expect(id).To(Equal(instanceID))
				Expect(data.InstanceMap[instanceID].PlanID).To(Equal("Existing"))
			})

			Context("create-service was given invalid JSON", func() {
				BeforeEach(func() {
					badJson := []byte("{this is not json")
					provisionDetails = brokerapi.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(badJson)}
				})

				It("errors", func() {
					Expect(err).To(Equal(brokerapi.ErrRawParamsInvalid))
				})

			})
			Context("create-service was given valid JSON but no 'share' key", func() {
				BeforeEach(func() {
					configuration := map[string]interface{}{"unknown key": "server:/some-share"}
					buf := &bytes.Buffer{}
					_ = json.NewEncoder(buf).Encode(configuration)
					provisionDetails = brokerapi.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}
				})

				It("errors", func() {
					Expect(err).To(Equal(errors.New("config requires a \"share\" key")))
				})
			})

			Context("when the service instance already exists with different details", func() {
				// enclosing context creates initial instance
				JustBeforeEach(func() {
					provisionDetails.ServiceID = "different-service-id"
					_, err = broker.Provision(ctx, "some-instance-id", provisionDetails, true)
				})

				It("should error", func() {
					Expect(err).To(Equal(brokerapi.ErrInstanceAlreadyExists))
				})
			})
		})

		Context(".Deprovision", func() {
			var (
				instanceID       string
				asyncAllowed     bool
				provisionDetails brokerapi.ProvisionDetails

				err error
			)

			BeforeEach(func() {
				instanceID = "some-instance-id"
				provisionDetails = brokerapi.ProvisionDetails{PlanID: "Existing"}
				asyncAllowed = true

			})

			JustBeforeEach(func() {
				_, err = broker.Deprovision(ctx, instanceID, brokerapi.DeprovisionDetails{}, asyncAllowed)
			})

			Context("when the instance does not exist", func() {
				BeforeEach(func() {
					instanceID = "does-not-exist"
				})

				It("should fail", func() {
					Expect(err).To(Equal(brokerapi.ErrInstanceDoesNotExist))
				})
			})

			Context("given an existing instance", func() {
				var (
					spec brokerapi.ProvisionedServiceSpec
				)

				BeforeEach(func() {
					instanceID = "some-instance-id"

					configuration := map[string]interface{}{"share": "server:/some-share"}
					buf := &bytes.Buffer{}
					_ = json.NewEncoder(buf).Encode(configuration)
					provisionDetails = brokerapi.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}
					asyncAllowed = false

					spec, err = broker.Provision(ctx, instanceID, provisionDetails, asyncAllowed)
					Expect(err).NotTo(HaveOccurred())
				})

				It("should succeed", func() {
					Expect(err).NotTo(HaveOccurred())
				})

				It("save state", func() {
					Expect(fakeStore.SaveCallCount()).To(Equal(2))
					_, data, id, _ := fakeStore.SaveArgsForCall(fakeStore.SaveCallCount() - 1)
					Expect(id).To(Equal(instanceID))
					_, exists := data.InstanceMap[instanceID]
					Expect(exists).To(BeFalse())
				})
			})

		})

		Context(".LastOperation", func() {
			It("errors", func() {
				_, err := broker.LastOperation(ctx, "non-existant", "provision")
				Expect(err).To(HaveOccurred())
			})
		})

		Context(".Bind", func() {
			var (
				instanceID  string
				bindDetails brokerapi.BindDetails

				uid, gid string
			)

			BeforeEach(func() {
				instanceID = "some-instance-id"
				uid = "1234"
				gid = "5678"

				configuration := map[string]interface{}{"share": "server:/some-share"}

				buf := &bytes.Buffer{}
				_ = json.NewEncoder(buf).Encode(configuration)
				provisionDetails := brokerapi.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}

				_, err := broker.Provision(ctx, instanceID, provisionDetails, false)
				Expect(err).NotTo(HaveOccurred())

				bindDetails = brokerapi.BindDetails{AppGUID: "guid", Parameters: map[string]interface{}{
					nfsbroker.Username: "principal name",
					nfsbroker.Secret:   "some keytab data",
					"uid":              uid,
					"gid":              gid,
				},
				}
			})

			It("passes `share` from create-service into `mountConfig.ip` on the bind response", func() {
				binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())
				mc := binding.VolumeMounts[0].Device.MountConfig
				share, ok := mc["source"].(string)
				Expect(ok).To(BeTrue())
				Expect(share).To(Equal(fmt.Sprintf("nfs://server:/some-share?uid=%s&gid=%s", uid, gid)))
			})

			Context("given the uid is not supplied", func() {
				BeforeEach(func() {
					bindDetails = brokerapi.BindDetails{AppGUID: "guid", Parameters: map[string]interface{}{
						nfsbroker.Username: "principal name",
						nfsbroker.Secret:   "some keytab data",
						"gid":              gid,
					},
					}
				})

				It("should return with an error", func() {
					_, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails)
					Expect(err).To(HaveOccurred())
				})
			})

			Context("given the gid is not supplied", func() {
				BeforeEach(func() {
					bindDetails = brokerapi.BindDetails{AppGUID: "guid", Parameters: map[string]interface{}{
						nfsbroker.Username: "principal name",
						nfsbroker.Secret:   "some keytab data",
						"uid":              uid,
					},
					}
				})

				It("should return with an error", func() {
					_, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails)
					Expect(err).To(HaveOccurred())
				})
			})

			It("includes empty credentials to prevent CAPI crash", func() {
				binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.Credentials).NotTo(BeNil())
			})

			It("uses the instance id in the default container path", func() {
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())
				Expect(binding.VolumeMounts[0].ContainerDir).To(Equal("/var/vcap/data/some-instance-id"))
			})

			It("flows container path through", func() {
				bindDetails.Parameters["mount"] = "/var/vcap/otherdir/something"
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())
				Expect(binding.VolumeMounts[0].ContainerDir).To(Equal("/var/vcap/otherdir/something"))
			})

			It("uses rw as its default mode", func() {
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())
				Expect(binding.VolumeMounts[0].Mode).To(Equal("rw"))
			})

			It("sets mode to `r` when readonly is true", func() {
				bindDetails.Parameters["readonly"] = true
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.VolumeMounts[0].Mode).To(Equal("r"))
			})

			It("should write state", func() {
				_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())

				_, data, _, _ := fakeStore.SaveArgsForCall(fakeStore.SaveCallCount() - 1)
				Expect(data.InstanceMap[instanceID].PlanID).To(Equal("Existing"))
			})

			It("errors if mode is not a boolean", func() {
				bindDetails.Parameters["readonly"] = ""
				_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
				Expect(err).To(Equal(brokerapi.ErrRawParamsInvalid))
			})

			It("fills in the driver name", func() {
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.VolumeMounts[0].Driver).To(Equal("nfsv3driver"))
			})

			It("fills in the volume id", func() {
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.VolumeMounts[0].Device.VolumeId).To(ContainSubstring("some-instance-id"))
			})

			Context("when the binding already exists", func() {
				BeforeEach(func() {
					_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
					Expect(err).NotTo(HaveOccurred())
				})

				It("doesn't error when binding the same details", func() {
					_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
					Expect(err).NotTo(HaveOccurred())
				})

				It("errors when binding different details", func() {
					bindDetails.AppGUID = "different"
					_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
					Expect(err).To(Equal(brokerapi.ErrBindingAlreadyExists))
				})
			})

			Context("given another binding with the same share", func() {
				var (
					err error
					bindSpec1 brokerapi.Binding
				)

				BeforeEach(func() {
					bindSpec1, err = broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
					Expect(err).NotTo(HaveOccurred())
				})

				Context("given different options", func() {
					var (
						bindSpec2 brokerapi.Binding
					)
					BeforeEach(func() {
						bindDetails.Parameters["uid"] = "3000"
						bindDetails.Parameters["gid"] = "3000"
						bindSpec2, err = broker.Bind(ctx, "some-instance-id", "binding-id-2", bindDetails)
						Expect(err).NotTo(HaveOccurred())
					})

					It("should issue a volume mount with a different volume ID", func() {
						Expect(bindSpec1.VolumeMounts[0].Device.VolumeId).NotTo(Equal(bindSpec2.VolumeMounts[0].Device.VolumeId))
					})
				})
			})

			It("errors when the service instance does not exist", func() {
				_, err := broker.Bind(ctx, "nonexistent-instance-id", "binding-id", brokerapi.BindDetails{AppGUID: "guid"})
				Expect(err).To(Equal(brokerapi.ErrInstanceDoesNotExist))
			})

			It("errors when the app guid is not provided", func() {
				_, err := broker.Bind(ctx, "some-instance-id", "binding-id", brokerapi.BindDetails{})
				Expect(err).To(Equal(brokerapi.ErrAppGuidNotProvided))
			})
		})

		Context(".Unbind", func() {
			var (
				instanceID  string
				err         error
				bindDetails brokerapi.BindDetails
			)

			BeforeEach(func() {
				instanceID = "some-instance-id"

				configuration := map[string]interface{}{"share": "server:/some-share"}

				buf := &bytes.Buffer{}
				_ = json.NewEncoder(buf).Encode(configuration)
				provisionDetails := brokerapi.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}

				_, err = broker.Provision(ctx, instanceID, provisionDetails, false)
				Expect(err).NotTo(HaveOccurred())

				bindDetails = brokerapi.BindDetails{AppGUID: "guid", Parameters: map[string]interface{}{nfsbroker.Username: "principal name", nfsbroker.Secret: "some keytab data", "uid": "1000", "gid": "1000"}}

				_, err = broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())
			})

			It("unbinds a bound service instance from an app", func() {
				err = broker.Unbind(ctx, "some-instance-id", "binding-id", brokerapi.UnbindDetails{})
				Expect(err).NotTo(HaveOccurred())
			})

			It("fails when trying to unbind a instance that has not been provisioned", func() {
				err = broker.Unbind(ctx, "some-other-instance-id", "binding-id", brokerapi.UnbindDetails{})
				Expect(err).To(Equal(brokerapi.ErrInstanceDoesNotExist))
			})

			It("fails when trying to unbind a binding that has not been bound", func() {
				err := broker.Unbind(ctx, "some-instance-id", "some-other-binding-id", brokerapi.UnbindDetails{})
				Expect(err).To(Equal(brokerapi.ErrBindingDoesNotExist))
			})
			It("should write state", func() {
				err := broker.Unbind(ctx, "some-instance-id", "binding-id", brokerapi.UnbindDetails{})
				Expect(err).NotTo(HaveOccurred())

				_, data, _, _ := fakeStore.SaveArgsForCall(fakeStore.SaveCallCount() - 1)
				Expect(data.InstanceMap[instanceID].PlanID).To(Equal("Existing"))
			})
		})

	})

	Context("when recreating", func() {
		var bindDetails brokerapi.BindDetails

		BeforeEach(func() {
			bindDetails = brokerapi.BindDetails{AppGUID: "guid", Parameters: map[string]interface{}{nfsbroker.Username: "principal name", nfsbroker.Secret: "some keytab data", "uid": "1000", "gid": "1000"}}
		})
		It("should be able to bind to previously created service", func() {
			fileContents := nfsbroker.DynamicState{
				InstanceMap: map[string]nfsbroker.ServiceInstance{
					"service-name": {
						Share: "server:/some-share",
					},
				},
				BindingMap: map[string]brokerapi.BindDetails{},
			}

			fakeStore.RestoreStub = func(logger lager.Logger, state *nfsbroker.DynamicState) error {
				*state = fileContents
				return nil
			}

			broker = nfsbroker.New(
				logger,
				"service-name", "service-id", "/fake-dir",
				fakeOs,
				nil,
				fakeStore,
			)

			_, err := broker.Bind(ctx, "service-name", "whatever", bindDetails)
			Expect(err).NotTo(HaveOccurred())
		})
	})

})
