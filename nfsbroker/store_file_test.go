package nfsbroker_test

import (
	"errors"

	"code.cloudfoundry.org/goshims/ioutilshim/ioutil_fake"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/nfsbroker/nfsbroker"
	"github.com/pivotal-cf/brokerapi"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("FileStore", func() {
	var (
		store      nfsbroker.Store
		fakeIoutil *ioutil_fake.FakeIoutil
		logger     lager.Logger
		state      nfsbroker.DynamicState
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test-broker")
		fakeIoutil = &ioutil_fake.FakeIoutil{}
		store = nfsbroker.NewFileStore("/tmp/whatever", fakeIoutil)
		state = nfsbroker.DynamicState{
			InstanceMap: map[string]nfsbroker.ServiceInstance{
				"service-name": {
					Share: "server:/some-share",
				},
			},
			BindingMap: map[string]brokerapi.BindDetails{},
		}
	})

	Describe("Restore", func() {
		var (
			err error
		)

		Context("when it succeeds", func() {
			BeforeEach(func() {
				fakeIoutil.ReadFileReturns([]byte(`{"InstanceMap":{},"BindingMap":{}}`), nil)
				err = store.Restore(logger)
			})

			It("reads the file", func() {
				Expect(fakeIoutil.ReadFileCallCount()).To(Equal(1))
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when the file system is failing", func() {
			BeforeEach(func() {
				fakeIoutil.ReadFileReturns(nil, errors.New("badness"))
				err = store.Restore(logger)
			})

			It("returns an error", func() {
				Expect(err).To(MatchError("badness"))
			})
		})

		Context("when there is junk in the file", func() {
			BeforeEach(func() {
				filecontents := "{serviceName: [some invalid state]}"
				fakeIoutil.ReadFileReturns([]byte(filecontents), nil)
				err = store.Restore(logger)
			})
			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Save", func() {
		var (
			err error
		)

		Context("when it succeeds", func() {
			BeforeEach(func() {
				fakeIoutil.WriteFileReturns(nil)
				err = store.Save(logger)
			})

			It("writes the file", func() {
				Expect(fakeIoutil.WriteFileCallCount()).To(Equal(1))
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when the file system is failing", func() {
			BeforeEach(func() {
				fakeIoutil.WriteFileReturns(errors.New("badness"))
				err = store.Save(logger)
			})

			It("returns an error", func() {
				Expect(err).To(MatchError("badness"))
			})
		})
	})

	Describe("Cleanup", func() {
		var (
			err error
		)

		Context("when it succeeds", func() {
			BeforeEach(func() {
				err = store.Cleanup()
			})

			It("doesn't error", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Describe("Create, Retrieve and Delete InstanceDetails", func() {
		var (
			instanceID         string
			err                error
			outInstanceDetails nfsbroker.ServiceInstance
			inInstanceDetails  nfsbroker.ServiceInstance
		)
		JustBeforeEach(func() {
			outInstanceDetails, err = store.RetrieveInstanceDetails(instanceID)
		})

		Context("when details not found", func() {
			BeforeEach(func() {
				instanceID = "garbage"
			})

			It("then will error", func() {
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when details found", func() {
			BeforeEach(func() {
				instanceID = "somethingGood"
				inInstanceDetails = nfsbroker.ServiceInstance{ServiceID: "sample-service"}
				store.CreateInstanceDetails(instanceID, inInstanceDetails)
			})
			It("then will find instance details", func() {
				Expect(outInstanceDetails).To(Equal(inInstanceDetails))
			})

			It("reports conflicts correctly", func() {
				Expect(store.IsInstanceConflict(instanceID, inInstanceDetails)).To(BeFalse())
				otherInstance := nfsbroker.ServiceInstance{ServiceID: "sample-service", PlanID: "foo"}
				Expect(store.IsInstanceConflict(instanceID, otherInstance)).To(BeTrue())
			})

			Context("when deleting", func() {
				JustBeforeEach(func() {
					err = store.DeleteInstanceDetails(instanceID)
				})
				It("then should not error", func() {
					Expect(err).ToNot(HaveOccurred())
				})
				It("then should not be able to delete again", func() {
					err = store.DeleteInstanceDetails(instanceID)
					Expect(err).To(HaveOccurred())
				})
			})

		})
		Describe("Create, Retrieve and Delete BindingDetails", func() {
			var (
				bindingID         string
				err               error
				outBindingDetails brokerapi.BindDetails
				inBindingDetails  brokerapi.BindDetails
			)
			JustBeforeEach(func() {
				outBindingDetails, err = store.RetrieveBindingDetails(bindingID)
			})

			Context("when details not found", func() {
				BeforeEach(func() {
					bindingID = "garbage"
				})

				It("then will error", func() {
					Expect(err).To(HaveOccurred())
				})
			})

			Context("when details found", func() {
				BeforeEach(func() {
					bindingID = "somethingGood"
					inBindingDetails = brokerapi.BindDetails{ServiceID: "sample-service", Parameters: map[string]interface{}{"ping": "pong"}}
					store.CreateBindingDetails(bindingID, inBindingDetails)
				})
				It("then will find binding details", func() {
					Expect(outBindingDetails.ServiceID).To(Equal(inBindingDetails.ServiceID))
				})

				It("reports conflicts correctly", func() {
					Expect(store.IsBindingConflict(bindingID, inBindingDetails)).To(BeFalse())
					otherBindingDetails := brokerapi.BindDetails{ServiceID: "sample-service", Parameters: map[string]interface{}{"foo": "foo"}}
					Expect(store.IsBindingConflict(bindingID, otherBindingDetails)).To(BeTrue())
					otherBindingDetails = brokerapi.BindDetails{ServiceID: "sample-service"}
					Expect(store.IsBindingConflict(bindingID, otherBindingDetails)).To(BeTrue())
					otherBindingDetails = brokerapi.BindDetails{ServiceID: "sample-service", Parameters: map[string]interface{}{}}
					Expect(store.IsBindingConflict(bindingID, otherBindingDetails)).To(BeTrue())
				})

				Context("when deleting", func() {
					JustBeforeEach(func() {
						err = store.DeleteBindingDetails(bindingID)
					})
					It("then should not error", func() {
						Expect(err).ToNot(HaveOccurred())
					})
					It("then should not be able to delete again", func() {
						err = store.DeleteBindingDetails(bindingID)
						Expect(err).To(HaveOccurred())
					})
				})

			})
		})
	})
})
