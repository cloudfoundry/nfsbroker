package nfsbroker_test

import (
	"bytes"
	"errors"

	"code.cloudfoundry.org/lager/lagertest"
	"github.com/pivotal-cf/brokerapi"

	"context"

	"encoding/json"

	"code.cloudfoundry.org/goshims/osshim/os_fake"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/nfsbroker/nfsbroker"
	"code.cloudfoundry.org/nfsbroker/nfsbroker/nfsbrokerfakes"
	"code.cloudfoundry.org/service-broker-store/brokerstore"
	"code.cloudfoundry.org/service-broker-store/brokerstore/brokerstorefakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Broker", func() {
	var (
		broker       *nfsbroker.Broker
		fakeOs       *os_fake.FakeOs
		logger       lager.Logger
		ctx          context.Context
		fakeStore    *brokerstorefakes.FakeStore
		fakeServices *nfsbrokerfakes.FakeServices
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test-broker")
		ctx = context.TODO()
		fakeOs = &os_fake.FakeOs{}
		fakeStore = &brokerstorefakes.FakeStore{}
		fakeServices = &nfsbrokerfakes.FakeServices{}
		fakeServices.ListReturns([]brokerapi.Service{
			{
				ID:            "nfs-service-id",
				Name:          "nfs",
				Description:   "Existing NFSv3 volumes",
				Bindable:      true,
				PlanUpdatable: false,
				Tags:          []string{"nfs"},
				Requires:      []brokerapi.RequiredPermission{"volume_mount"},
				Plans: []brokerapi.ServicePlan{
					{
						Name:        "Existing",
						ID:          "Existing",
						Description: "A preexisting filesystem",
					},
				},
			}, {
				ID:            "nfs-experimental-service-id",
				Name:          "nfs-experimental",
				Description:   "Experimental support for NFSv3 and v4",
				Bindable:      true,
				PlanUpdatable: false,
				Tags:          []string{"nfs", "experimental"},
				Requires:      []brokerapi.RequiredPermission{"volume_mount"},

				Plans: []brokerapi.ServicePlan{
					{
						Name:        "Existing",
						ID:          "Existing",
						Description: "A preexisting filesystem",
					},
				},
			},
		})
	})

	Context("when creating first time", func() {
		BeforeEach(func() {
			mounts := nfsbroker.NewNfsBrokerConfigDetails()
			mounts.ReadConf("sloppy_mount,allow_other,allow_root,multithread,default_permissions,fusenfs_uid,fusenfs_gid,uid,gid,version,username,password", "sloppy_mount:false")
			broker = nfsbroker.New(
				logger,
				fakeServices,
				"/fake-dir",
				fakeOs,
				nil,
				fakeStore,
				nfsbroker.NewNfsBrokerConfig(mounts),
			)
		})

		Context(".Services", func() {
			It("returns the service catalog as appropriate", func() {
				results, err := broker.Services(ctx)
				Expect(err).NotTo(HaveOccurred())
				result := results[0]
				Expect(result.ID).To(Equal("nfs-service-id"))
				Expect(result.Name).To(Equal("nfs"))
				Expect(result.Description).To(Equal("Existing NFSv3 volumes"))
				Expect(result.Bindable).To(Equal(true))
				Expect(result.PlanUpdatable).To(Equal(false))
				Expect(result.Tags).To(ContainElement("nfs"))
				Expect(result.Requires).To(ContainElement(brokerapi.RequiredPermission("volume_mount")))

				Expect(result.Plans[0].Name).To(Equal("Existing"))
				Expect(result.Plans[0].ID).To(Equal("Existing"))
				Expect(result.Plans[0].Description).To(Equal("A preexisting filesystem"))

				result = results[1]
				Expect(result.ID).To(Equal("nfs-experimental-service-id"))
				Expect(result.Name).To(Equal("nfs-experimental"))
				Expect(result.Tags).To(ContainElement("nfs"))
				Expect(result.Requires).To(ContainElement(brokerapi.RequiredPermission("volume_mount")))

				Expect(results).To(HaveLen(2))
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

				configuration := map[string]interface{}{"share": "server/some-share"}
				buf := &bytes.Buffer{}
				_ = json.NewEncoder(buf).Encode(configuration)
				provisionDetails = brokerapi.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}
				asyncAllowed = false
				fakeStore.RetrieveInstanceDetailsReturns(brokerstore.ServiceInstance{}, errors.New("not found"))
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
				Expect(fakeStore.SaveCallCount()).Should(BeNumerically(">", 0))
			})

			Context("when create service json contains uid and gid", func() {
				BeforeEach(func() {
					configuration := map[string]interface{}{"share": "server/some-share", "uid": "1", "gid": 2}
					buf := &bytes.Buffer{}
					_ = json.NewEncoder(buf).Encode(configuration)
					provisionDetails = brokerapi.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}
				})
				It("should write uid and gid into state", func() {
					count := fakeStore.CreateInstanceDetailsCallCount()
					Expect(count).To(BeNumerically(">", 0))
					_, details := fakeStore.CreateInstanceDetailsArgsForCall(count - 1)
					fp := details.ServiceFingerPrint.(map[string]interface{})
					Expect(fp).NotTo(BeNil())
					Expect(fp).To(HaveKeyWithValue("uid", "1"))
					Expect(fp).To(HaveKey("gid"))
				})
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

			Context("create-service was given a server share with colon after server", func() {
				BeforeEach(func() {
					configuration := map[string]interface{}{"share": "server:/some-share"}
					buf := &bytes.Buffer{}
					_ = json.NewEncoder(buf).Encode(configuration)
					provisionDetails = brokerapi.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}
				})

				It("errors", func() {
					Expect(err).To(Equal(errors.New("syntax error for share: no colon allowed after server")))
				})
			})

			Context("create-service was given a server share with colon after nfs directory", func() {
				BeforeEach(func() {
					configuration := map[string]interface{}{"share": "server/some-share:dir/"}
					buf := &bytes.Buffer{}
					_ = json.NewEncoder(buf).Encode(configuration)
					provisionDetails = brokerapi.ProvisionDetails{PlanID: "Existing", RawParameters: json.RawMessage(buf.Bytes())}
				})

				It("should not error", func() {
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when the service instance already exists with the same details", func() {
				BeforeEach(func() {
					fakeStore.IsInstanceConflictReturns(false)
				})

				It("should not error", func() {
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when the service instance already exists with different details", func() {
				BeforeEach(func() {
					fakeStore.IsInstanceConflictReturns(true)
				})

				It("should error", func() {
					Expect(err).To(Equal(brokerapi.ErrInstanceAlreadyExists))
				})
			})

			Context("when the service instance creation fails", func() {
				BeforeEach(func() {
					fakeStore.CreateInstanceDetailsReturns(errors.New("badness"))
				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
				})
			})

			Context("when the save fails", func() {
				BeforeEach(func() {
					fakeStore.SaveReturns(errors.New("badness"))
				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
				})
			})
		})

		Context(".Deprovision", func() {
			var (
				instanceID   string
				asyncAllowed bool
				err          error
			)

			BeforeEach(func() {
				instanceID = "some-instance-id"
				asyncAllowed = true

			})

			JustBeforeEach(func() {
				_, err = broker.Deprovision(ctx, instanceID, brokerapi.DeprovisionDetails{}, asyncAllowed)
			})

			Context("when the instance does not exist", func() {
				BeforeEach(func() {
					instanceID = "does-not-exist"
					fakeStore.RetrieveInstanceDetailsReturns(brokerstore.ServiceInstance{}, brokerapi.ErrInstanceDoesNotExist)
				})

				It("should fail", func() {
					Expect(err).To(Equal(brokerapi.ErrInstanceDoesNotExist))
				})
			})

			Context("given an existing instance", func() {
				var (
					previousSaveCallCount int
				)

				BeforeEach(func() {
					instanceID = "some-instance-id"

					configuration := map[string]interface{}{"share": "server:/some-share"}
					buf := &bytes.Buffer{}
					_ = json.NewEncoder(buf).Encode(configuration)
					asyncAllowed = false
					fakeStore.RetrieveInstanceDetailsReturns(brokerstore.ServiceInstance{ServiceID: instanceID}, nil)
					previousSaveCallCount = fakeStore.SaveCallCount()
				})

				It("should succeed", func() {
					Expect(err).NotTo(HaveOccurred())
				})

				It("save state", func() {
					Expect(fakeStore.SaveCallCount()).To(Equal(previousSaveCallCount + 1))
				})

				Context("when deletion of the instance fails", func() {
					BeforeEach(func() {
						fakeStore.DeleteInstanceDetailsReturns(errors.New("badness"))
					})

					It("should error", func() {
						Expect(err).To(HaveOccurred())
					})
				})
			})

			Context("when the save fails", func() {
				BeforeEach(func() {
					fakeStore.SaveReturns(errors.New("badness"))
				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
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
				instanceID, serviceID string
				bindDetails           brokerapi.BindDetails
				bindParameters        map[string]interface{}

				uid, gid string
			)

			BeforeEach(func() {
				instanceID = "some-instance-id"
				serviceID = "some-service-id"
				uid = "1234"
				gid = "5678"

				serviceInstance := brokerstore.ServiceInstance{
					ServiceID: serviceID,
					ServiceFingerPrint: map[string]interface{}{
						nfsbroker.SHARE_KEY: "server:/some-share",
					},
				}

				fakeStore.RetrieveInstanceDetailsReturns(serviceInstance, nil)
				fakeStore.RetrieveBindingDetailsReturns(brokerapi.BindDetails{}, errors.New("yar"))

				bindParameters = map[string]interface{}{
					nfsbroker.Username: "principal name",
					nfsbroker.Secret:   "some keytab data",
					"uid":              uid,
					"gid":              gid,
				}
				bindMessage, err := json.Marshal(bindParameters)
				Expect(err).NotTo(HaveOccurred())

				bindDetails = brokerapi.BindDetails{
					AppGUID:       "guid",
					RawParameters: bindMessage,
				}
			})

			It("passes `share` from create-service into `mountConfig.ip` on the bind response", func() {
				binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())

				mc := binding.VolumeMounts[0].Device.MountConfig

				v, ok := mc["source"].(string)
				Expect(ok).To(BeTrue())
				Expect(v).To(Equal("nfs://server:/some-share"))
				v, ok = mc["uid"].(string)
				Expect(ok).To(BeTrue())
				Expect(v).To(Equal(uid))
				v, ok = mc["gid"].(string)
				Expect(ok).To(BeTrue())
				Expect(v).To(Equal(gid))
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
				var err error
				bindParameters["mount"] = "/var/vcap/otherdir/something"
				bindDetails.RawParameters, err = json.Marshal(bindParameters)
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())
				Expect(binding.VolumeMounts[0].ContainerDir).To(Equal("/var/vcap/otherdir/something"))
			})

			It("uses rw as its default mode", func() {
				binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())
				Expect(binding.VolumeMounts[0].Mode).To(Equal("rw"))
			})

			//It("sets mode to `r` when readonly is true", func() {
			//	bindDetails.Parameters["readonly"] = true
			//	binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
			//	Expect(err).NotTo(HaveOccurred())
			//
			//	Expect(binding.VolumeMounts[0].Mode).To(Equal("r"))
			//	Expect(binding.VolumeMounts[0].Device.MountConfig["readonly"]).To(Equal(true))
			//})

			It("should write state", func() {
				previousSaveCallCount := fakeStore.SaveCallCount()
				_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeStore.SaveCallCount()).To(Equal(previousSaveCallCount + 1))
			})

			It("errors if mode is not a boolean", func() {
				var err error
				bindParameters["readonly"] = ""
				bindDetails.RawParameters, err = json.Marshal(bindParameters)
				_, err = broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
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

			Context("when the service instance contains uid and gid", func() {
				BeforeEach(func() {
					serviceInstance := brokerstore.ServiceInstance{
						ServiceID: serviceID,
						ServiceFingerPrint: map[string]interface{}{
							nfsbroker.SHARE_KEY: "server:/some-share",
							"uid":               "1",
							"gid":               2,
						},
					}
					fakeStore.RetrieveInstanceDetailsReturns(serviceInstance, nil)
				})

				It("should favor the values in the bind configuration", func() {
					binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails)
					Expect(err).NotTo(HaveOccurred())

					mc := binding.VolumeMounts[0].Device.MountConfig

					v, ok := mc["uid"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal(uid))
					v, ok = mc["gid"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal(gid))
				})

				Context("when the bind operation doesn't pass configuration", func() {
					BeforeEach(func() {
						bindDetails = brokerapi.BindDetails{
							AppGUID:       "guid",
							RawParameters: []byte(""),
						}
					})

					It("should use uid and gid from the service instance configuration", func() {
						binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
						Expect(err).NotTo(HaveOccurred())
						mc := binding.VolumeMounts[0].Device.MountConfig

						v, ok := mc["uid"].(string)
						Expect(ok).To(BeTrue())
						Expect(v).To(Equal("1"))
						v, ok = mc["gid"].(string)
						Expect(ok).To(BeTrue())
						Expect(v).To(Equal("2"))
					})
				})
			})

			Context("when the service instance contains username and password", func() {
				BeforeEach(func() {
					serviceInstance := brokerstore.ServiceInstance{
						ServiceID: serviceID,
						ServiceFingerPrint: map[string]interface{}{
							nfsbroker.SHARE_KEY: "server:/some-share",
							"username":          "some-instance-username",
							"password":          "some-instance-password",
						},
					}
					fakeStore.RetrieveInstanceDetailsReturns(serviceInstance, nil)
					bindDetails = brokerapi.BindDetails{
						AppGUID:       "guid",
						RawParameters: []byte(`{"username":"some-bind-username","password":"some-bind-password"}`),
					}
				})

				It("should favor the values in the bind configuration", func() {
					binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails)
					Expect(err).NotTo(HaveOccurred())

					mc := binding.VolumeMounts[0].Device.MountConfig

					v, ok := mc["username"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal("some-bind-username"))
					v, ok = mc["password"].(string)
					Expect(ok).To(BeTrue())
					Expect(v).To(Equal("some-bind-password"))
				})

				Context("when the bind operation doesn't pass configuration", func() {
					BeforeEach(func() {
						bindDetails = brokerapi.BindDetails{
							AppGUID:       "guid",
							RawParameters: []byte(""),
						}
					})

					It("should use uid and gid from the service instance configuration", func() {
						binding, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
						Expect(err).NotTo(HaveOccurred())
						mc := binding.VolumeMounts[0].Device.MountConfig

						v, ok := mc["username"].(string)
						Expect(ok).To(BeTrue())
						Expect(v).To(Equal("some-instance-username"))
						v, ok = mc["password"].(string)
						Expect(ok).To(BeTrue())
						Expect(v).To(Equal("some-instance-password"))
					})
				})
			})

			Context("when the bind operation doesn't pass configuration", func() {
				BeforeEach(func() {
					bindDetails = brokerapi.BindDetails{
						AppGUID:       "guid",
						RawParameters: []byte(""),
					}
				})

				It("should succeed", func() {
					_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when the bind operation passes empty configuration", func() {
				BeforeEach(func() {
					bindDetails = brokerapi.BindDetails{
						AppGUID:       "guid",
						RawParameters: []byte("{}"),
					}
				})

				It("should succeed", func() {
					_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when the service id is an experimental service", func() {
				BeforeEach(func() {
					fakeStore.RetrieveInstanceDetailsReturns(
						brokerstore.ServiceInstance{
							ServiceID: "nfs-experimental-service-id",
							ServiceFingerPrint: map[string]interface{}{
								nfsbroker.SHARE_KEY: "server:/some-share",
							},
						}, nil)
				})

				It("includes 'experimental' in the service binding mount config", func() {
					binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails)
					Expect(err).NotTo(HaveOccurred())

					mc := binding.VolumeMounts[0].Device.MountConfig
					_, ok := mc[nfsbroker.EXPERIMENTAL_TAG]

					Expect(ok).To(BeTrue())
				})
			})

			Context("when using nfs version", func() {
				BeforeEach(func() {

					serviceInstance := brokerstore.ServiceInstance{
						ServiceID: "nfs-experimental-service-id",
						ServiceFingerPrint: map[string]interface{}{
							nfsbroker.SHARE_KEY:   "server:/some-share",
							nfsbroker.VERSION_KEY: "4.1",
						},
					}

					fakeStore.RetrieveInstanceDetailsReturns(serviceInstance, nil)
				})

				It("includes version in the service binding mount config", func() {
					binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails)
					Expect(err).NotTo(HaveOccurred())

					mc := binding.VolumeMounts[0].Device.MountConfig
					version, ok := mc["version"]

					Expect(ok).To(BeTrue())
					Expect(version).To(Equal("4.1"))
				})
			})

			Context("when the binding already exists", func() {

				It("doesn't error when binding the same details", func() {
					fakeStore.IsBindingConflictReturns(false)
					_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
					Expect(err).NotTo(HaveOccurred())
				})

				It("errors when binding different details", func() {
					fakeStore.IsBindingConflictReturns(true)
					_, err := broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
					Expect(err).To(Equal(brokerapi.ErrBindingAlreadyExists))
				})
			})

			Context("given another binding with the same share", func() {
				var (
					err       error
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
						var err error
						bindParameters["uid"] = "3000"
						bindParameters["gid"] = "3000"
						bindDetails.RawParameters, err = json.Marshal(bindParameters)
						bindSpec2, err = broker.Bind(ctx, "some-instance-id", "binding-id-2", bindDetails)
						Expect(err).NotTo(HaveOccurred())
					})

					It("should issue a volume mount with a different volume ID", func() {
						Expect(bindSpec1.VolumeMounts[0].Device.VolumeId).NotTo(Equal(bindSpec2.VolumeMounts[0].Device.VolumeId))
					})
				})
			})

			Context("when the binding cannot be stored", func() {
				var (
					err error
				)

				BeforeEach(func() {
					fakeStore.CreateBindingDetailsReturns(errors.New("badness"))
					_, err = broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)

				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
				})

			})

			Context("when the save fails", func() {
				var (
					err error
				)
				BeforeEach(func() {
					fakeStore.SaveReturns(errors.New("badness"))
					_, err = broker.Bind(ctx, "some-instance-id", "binding-id", bindDetails)
				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
				})
			})

			It("errors when the service instance does not exist", func() {
				fakeStore.RetrieveInstanceDetailsReturns(brokerstore.ServiceInstance{}, errors.New("Awesome!"))
				_, err := broker.Bind(ctx, "nonexistent-instance-id", "binding-id", brokerapi.BindDetails{AppGUID: "guid"})
				Expect(err).To(Equal(brokerapi.ErrInstanceDoesNotExist))
			})

			It("errors when the app guid is not provided", func() {
				_, err := broker.Bind(ctx, "some-instance-id", "binding-id", brokerapi.BindDetails{})
				Expect(err).To(Equal(brokerapi.ErrAppGuidNotProvided))
			})

			Context("given allowed and default parameters are empty", func() {
				BeforeEach(func() {
					mounts := nfsbroker.NewNfsBrokerConfigDetails()
					mounts.ReadConf("", "")
					broker = nfsbroker.New(
						logger,
						fakeServices,
						"/fake-dir",
						fakeOs,
						nil,
						fakeStore,
						nfsbroker.NewNfsBrokerConfig(mounts),
					)
				})

				Context("given allow_root=true is supplied", func() {
					BeforeEach(func() {
						bindParameters := map[string]interface{}{
							nfsbroker.Username: "principal name",
							nfsbroker.Secret:   "some keytab data",
							"allow_root":       true,
						}
						bindMessage, err := json.Marshal(bindParameters)
						Expect(err).NotTo(HaveOccurred())
						bindDetails = brokerapi.BindDetails{AppGUID: "guid", RawParameters: bindMessage}
					})

					It("should return with an error", func() {
						_, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails)
						Expect(err).To(HaveOccurred())
					})
				})
			})

			Context("given allowed and default parameters are empty, except for mount default with sloppy_mount=true is supplied ", func() {
				BeforeEach(func() {
					mounts := nfsbroker.NewNfsBrokerConfigDetails()
					mounts.ReadConf("", "sloppy_mount:true")
					broker = nfsbroker.New(
						logger,
						fakeServices,
						"/fake-dir",
						fakeOs,
						nil,
						fakeStore,
						nfsbroker.NewNfsBrokerConfig(mounts),
					)
				})

				Context("given allow_root=true is supplied", func() {
					BeforeEach(func() {
						bindParameters := map[string]interface{}{
							nfsbroker.Username: "principal name",
							nfsbroker.Secret:   "some keytab data",
							"allow_root":       true,
						}
						bindMessage, err := json.Marshal(bindParameters)
						Expect(err).NotTo(HaveOccurred())
						bindDetails = brokerapi.BindDetails{AppGUID: "guid", RawParameters: bindMessage}
					})

					It("does not pass allow_root option through", func() {
						binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails)
						Expect(err).NotTo(HaveOccurred())

						mc := binding.VolumeMounts[0].Device.MountConfig
						_, ok := mc["allow_root"]

						Expect(ok).To(BeFalse())
					})
				})
			})

			Context("given default parameters are empty, allowed parameters contain allow_root", func() {
				BeforeEach(func() {
					mounts := nfsbroker.NewNfsBrokerConfigDetails()
					mounts.ReadConf("allow_root", "")
					broker = nfsbroker.New(
						logger,
						fakeServices,
						"/fake-dir",
						fakeOs,
						nil,
						fakeStore,
						nfsbroker.NewNfsBrokerConfig(mounts),
					)
				})

				Context("given allow_root=true is supplied", func() {
					BeforeEach(func() {
						bindParameters := map[string]interface{}{
							nfsbroker.Username: "principal name",
							nfsbroker.Secret:   "some keytab data",
							"allow_root":       true,
						}
						bindMessage, err := json.Marshal(bindParameters)
						Expect(err).NotTo(HaveOccurred())
						bindDetails = brokerapi.BindDetails{AppGUID: "guid", RawParameters: bindMessage}
					})

					It("passes allow_root=true option through", func() {
						binding, err := broker.Bind(ctx, instanceID, "binding-id", bindDetails)
						Expect(err).NotTo(HaveOccurred())

						mc := binding.VolumeMounts[0].Device.MountConfig
						ar, ok := mc["allow_root"].(string)

						Expect(ok).To(BeTrue())
						Expect(ar).To(Equal("true"))
					})
				})
			})
		})

		Context(".Unbind", func() {
			var (
				bindDetails brokerapi.BindDetails
			)

			BeforeEach(func() {
				bindParameters := map[string]interface{}{
					nfsbroker.Username: "principal name",
					nfsbroker.Secret:   "some keytab data",
					"uid":              "1000",
					"gid":              "1000",
				}
				bindMessage, err := json.Marshal(bindParameters)
				Expect(err).NotTo(HaveOccurred())
				bindDetails = brokerapi.BindDetails{AppGUID: "guid", RawParameters: bindMessage}

				fakeStore.RetrieveBindingDetailsReturns(bindDetails, nil)
			})
			It("unbinds a bound service instance from an app", func() {
				err := broker.Unbind(ctx, "some-instance-id", "binding-id", brokerapi.UnbindDetails{})
				Expect(err).NotTo(HaveOccurred())
			})

			It("fails when trying to unbind a instance that has not been provisioned", func() {
				fakeStore.RetrieveInstanceDetailsReturns(brokerstore.ServiceInstance{}, errors.New("Shazaam!"))
				err := broker.Unbind(ctx, "some-other-instance-id", "binding-id", brokerapi.UnbindDetails{})
				Expect(err).To(Equal(brokerapi.ErrInstanceDoesNotExist))
			})

			It("fails when trying to unbind a binding that has not been bound", func() {
				fakeStore.RetrieveBindingDetailsReturns(brokerapi.BindDetails{}, errors.New("Hooray!"))
				err := broker.Unbind(ctx, "some-instance-id", "some-other-binding-id", brokerapi.UnbindDetails{})
				Expect(err).To(Equal(brokerapi.ErrBindingDoesNotExist))
			})
			It("should write state", func() {
				previousCallCount := fakeStore.SaveCallCount()
				err := broker.Unbind(ctx, "some-instance-id", "binding-id", brokerapi.UnbindDetails{})
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeStore.SaveCallCount()).To(Equal(previousCallCount + 1))
			})

			Context("when the save fails", func() {
				BeforeEach(func() {
					fakeStore.SaveReturns(errors.New("badness"))
				})

				It("should error", func() {
					err := broker.Unbind(ctx, "some-instance-id", "binding-id", brokerapi.UnbindDetails{})
					Expect(err).To(HaveOccurred())
				})
			})

			Context("when deletion of the binding details fails", func() {
				BeforeEach(func() {
					fakeStore.DeleteBindingDetailsReturns(errors.New("badness"))
				})

				It("should error", func() {
					err := broker.Unbind(ctx, "some-instance-id", "binding-id", brokerapi.UnbindDetails{})
					Expect(err).To(HaveOccurred())
				})
			})
		})
	})
})
