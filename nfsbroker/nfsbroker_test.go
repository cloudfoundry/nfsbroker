package nfsbroker_test

import (
	"bytes"
	"errors"

	"code.cloudfoundry.org/lager/lagertest"
	"github.com/pivotal-cf/brokerapi"

	"context"

	"encoding/json"

	"code.cloudfoundry.org/goshims/ioutilshim/ioutil_fake"
	"code.cloudfoundry.org/goshims/osshim/os_fake"
	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry-community/nfsbroker/nfsbroker"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type dynamicState struct {
	InstanceMap map[string]nfsbroker.ServiceInstance
	BindingMap  map[string]brokerapi.BindDetails
}

var _ = Describe("Broker", func() {
	var (
		broker     *nfsbroker.Broker
		fakeOs     *os_fake.FakeOs
		fakeIoutil *ioutil_fake.FakeIoutil
		logger     lager.Logger
		ctx        context.Context
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test-broker")
		ctx = context.TODO()
		fakeOs = &os_fake.FakeOs{}
		fakeIoutil = &ioutil_fake.FakeIoutil{}

	})

	Context("when creating first time", func() {
		BeforeEach(func() {
			broker = nfsbroker.New(
				logger,
				"service-name", "service-id", "/fake-dir",
				fakeOs,
				fakeIoutil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
			)
		})

		Context(".Services", func() {
			It("returns the service catalog as appropriate", func() {
				result := broker.Services(ctx)[0]
				Expect(result.ID).To(Equal("service-id"))
				Expect(result.Name).To(Equal("service-name"))
				Expect(result.Description).To(Equal("NFS volumes secured with Kerberos (see: https://example.com/knfs-volume-release/)"))
				Expect(result.Bindable).To(Equal(true))
				Expect(result.PlanUpdatable).To(Equal(false))
				Expect(result.Tags).To(ContainElement("knfs"))
				Expect(result.Requires).To(ContainElement(brokerapi.RequiredPermission("volume_mount")))

				Expect(result.Plans[0].Name).To(Equal("Existing"))
				Expect(result.Plans[0].ID).To(Equal("Existing"))
				Expect(result.Plans[0].Description).To(Equal("a filesystem you have already provisioned by contacting <URL>"))
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
				_, data, _ := fakeIoutil.WriteFileArgsForCall(fakeIoutil.WriteFileCallCount() - 1)
				Expect(string(data)).To(Equal(`{"InstanceMap":{"some-instance-id":{"service_id":"","plan_id":"Existing","organization_guid":"","space_guid":"","Share":"server:/some-share"}},"BindingMap":{}}`))
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
				provisionDetails = brokerapi.ProvisionDetails{PlanID: "generalPurpose"}
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
			)

			BeforeEach(func() {
				instanceID = "some-instance-id"

				configuration := map[string]interface{}{"share": "server:/some-share"}

				buf := &bytes.Buffer{}
				_ = json.NewEncoder(buf).Encode(configuration)
				provisionDetails := brokerapi.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}

				_, err := broker.Provision(ctx, instanceID, provisionDetails, false)
				Expect(err).NotTo(HaveOccurred())

				bindDetails = brokerapi.BindDetails{AppGUID: "guid", Parameters: map[string]interface{}{nfsbroker.Username: "principal name", nfsbroker.Secret: "some keytab data"}}
			})

			It("passes `share` from create-service into `mountConfig.ip` on the bind response", func() {
				binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())
				mc := binding.VolumeMounts[0].Device.MountConfig
				ip, ok := mc["ip"]
				Expect(ok).To(BeTrue())
				Expect(ip).To(Equal("server:/some-share"))

			})

			It("enforces that kerberosPrincipal, kerberosKeytab were supplied on the bind request", func() {
				bindDetails.Parameters = make(map[string]interface{})
				_, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails)
				Expect(err).To(HaveOccurred())

				bindDetails.Parameters = map[string]interface{}{nfsbroker.Username: "hello"}
				_, err = broker.Bind(ctx, instanceID, "binding-id", bindDetails)
				Expect(err).To(HaveOccurred())

				bindDetails.Parameters = map[string]interface{}{nfsbroker.Secret: "hello"}
				_, err = broker.Bind(ctx, instanceID, "binding-id", bindDetails)
				Expect(err).To(HaveOccurred())

			})

			It("passes kerberosPrincipal, kerberosKeytab in to MountConfig on the bind response", func() {
				binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())
				mc := binding.VolumeMounts[0].Device.MountConfig

				user, ok := mc[nfsbroker.Username]
				Expect(ok).To(BeTrue())
				Expect(user).To(Equal("principal name"))

				secret, ok := mc[nfsbroker.Secret]
				Expect(ok).To(BeTrue())
				Expect(secret).To(Equal("some keytab data"))
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

				_, data, _ := fakeIoutil.WriteFileArgsForCall(fakeIoutil.WriteFileCallCount() - 1)
				Expect(string(data)).To(Equal(`{"InstanceMap":{"some-instance-id":{"service_id":"","plan_id":"Existing","organization_guid":"","space_guid":"","Share":"server:/some-share"}},"BindingMap":{"binding-id":{"app_guid":"guid","plan_id":"","service_id":"","parameters":{"kerberosKeytab":"some keytab data","kerberosPrincipal":"principal name"}}}}`))
			})

			It("errors if mode is not a boolean", func() {
				bindDetails.Parameters["readonly"] = ""
				_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
				Expect(err).To(Equal(brokerapi.ErrRawParamsInvalid))
			})

			It("fills in the driver name", func() {
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.VolumeMounts[0].Driver).To(Equal("knfsdriver"))
			})

			It("fills in the group id", func() {
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())

				Expect(binding.VolumeMounts[0].Device.VolumeId).To(Equal("some-instance-id"))
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

				bindDetails = brokerapi.BindDetails{AppGUID: "guid", Parameters: map[string]interface{}{nfsbroker.Username: "principal name", nfsbroker.Secret: "some keytab data"}}

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

				_, data, _ := fakeIoutil.WriteFileArgsForCall(fakeIoutil.WriteFileCallCount() - 1)
				Expect(string(data)).To(Equal(`{"InstanceMap":{"some-instance-id":{"service_id":"","plan_id":"Existing","organization_guid":"","space_guid":"","Share":"server:/some-share"}},"BindingMap":{}}`))
			})
		})

	})

	Context("when recreating", func() {
		var bindDetails brokerapi.BindDetails

		BeforeEach(func() {
			bindDetails = brokerapi.BindDetails{AppGUID: "guid", Parameters: map[string]interface{}{nfsbroker.Username: "principal name", nfsbroker.Secret: "some keytab data"}}
		})
		It("should be able to bind to previously created service", func() {
			fileContents, err := json.Marshal(dynamicState{
				InstanceMap: map[string]nfsbroker.ServiceInstance{
					"service-name": {
						Share: "server:/some-share",
					},
				},
				BindingMap: map[string]brokerapi.BindDetails{},
			})
			Expect(err).NotTo(HaveOccurred())
			fakeIoutil.ReadFileReturns(fileContents, nil)

			broker = nfsbroker.New(
				logger,
				"service-name", "service-id", "/fake-dir",
				fakeOs,
				fakeIoutil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
			)

			_, err = broker.Bind(ctx, "service-name", "whatever", bindDetails)
			Expect(err).NotTo(HaveOccurred())
		})

		It("shouldn't be able to bind to service from invalid state file", func() {
			filecontents := "{serviceName: [some invalid state]}"
			fakeIoutil.ReadFileReturns([]byte(filecontents[:]), nil)

			broker = nfsbroker.New(
				logger,
				"service-name", "service-id", "/fake-dir",
				fakeOs,
				fakeIoutil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
			)

			_, err := broker.Bind(ctx, "service-name", "whatever", bindDetails)
			Expect(err).To(HaveOccurred())
		})
	})

})
