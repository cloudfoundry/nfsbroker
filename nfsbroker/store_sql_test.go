package nfsbroker_test

import (
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/nfsbroker/nfsbroker"
	"github.com/pivotal-cf/brokerapi"

	"code.cloudfoundry.org/goshims/sqlshim/sql_fake"
	"code.cloudfoundry.org/nfsbroker/nfsbrokerfakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/DATA-DOG/go-sqlmock.v1"
	"reflect"
	"encoding/json"
)

var _ = Describe("SqlStore", func() {
	var (
		store nfsbroker.Store
		logger lager.Logger
		state nfsbroker.DynamicState
		fakeSqlDb = &sql_fake.FakeSqlDB{}
		fakeVariant = &nfsbrokerfakes.FakeSqlVariant{}
		err error
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test-broker")
		fakeVariant.ConnectReturns(fakeSqlDb, nil)
		fakeVariant.FlavorifyStub = func(query string) string {
			return query
		}
		store, err = nfsbroker.NewSqlStoreWithVariant(logger, fakeVariant)
		Expect(err).ToNot(HaveOccurred())
		state = nfsbroker.DynamicState{
			InstanceMap: map[string]nfsbroker.ServiceInstance{
				"service-name": {
					Share: "server:/some-share",
				},
			},
			BindingMap: map[string]brokerapi.BindDetails{},
		}
	})

	It("should open a db connection", func() {
		Expect(fakeVariant.ConnectCallCount()).To(BeNumerically(">=", 1))
	})

	It("should create tables if they don't exist", func() {
		Expect(fakeSqlDb.ExecCallCount()).To(BeNumerically(">=", 2))
		Expect(fakeSqlDb.ExecArgsForCall(0)).To(ContainSubstring("CREATE TABLE IF NOT EXISTS service_instances"))
		Expect(fakeSqlDb.ExecArgsForCall(1)).To(ContainSubstring("CREATE TABLE IF NOT EXISTS service_bindings"))
	})

	Describe("Restore", func() {
		BeforeEach(func() {
			err = store.Restore(logger)
		})

		It("this should be a noop", func() {
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Save", func() {
		BeforeEach(func() {
			err = store.Save(logger)
		})

		It("this should be a noop", func() {
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Cleanup", func() {
		BeforeEach(func() {
			err = store.Cleanup()
		})

		It("this should be a noop", func() {
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("RetrieveInstanceDetails", func() {
		var serviceID, planID, orgGUID, spaceGUID, share string
		var err error
		var serviceInstance nfsbroker.ServiceInstance

		db, mock, err := sqlmock.New()
		sqlStore := nfsbroker.SqlStore{Database:nfsbrokerfakes.FakeSQLMockConnection{db},
			StoreType:"mysql"}

		Context("When the instance exists", func() {
			BeforeEach(func() {
				Expect(err).NotTo(HaveOccurred())
				serviceID = "instance_123"
				planID = "plan_123"
				orgGUID = "org_123"
				spaceGUID = "space_123"
				share = "share_123"

				columns := []string{"ServiceID", "PlanID", "OrgGUID", "SpaceGUID", "Share"}
				rows := sqlmock.NewRows(columns)
				rows.AddRow(serviceID,planID,orgGUID,spaceGUID,share)

				mock.ExpectQuery("SELECT service_instances.id FROM service_instances WHERE service_instance.id = ?").WithArgs(serviceID).WillReturnRows(rows)
			})
			JustBeforeEach(func() {

				serviceInstance, err = sqlStore.RetrieveInstanceDetails(serviceID)
			})
			It("should return the instance", func() {
				Expect(err).To(BeNil())
				Expect(mock.ExpectationsWereMet()).Should(Succeed())
				Expect(serviceInstance.ServiceID).To(Equal(serviceID))
				Expect(serviceInstance.PlanID).To(Equal(planID))
				Expect(serviceInstance.OrganizationGUID).To(Equal(orgGUID))
				Expect(serviceInstance.SpaceGUID).To(Equal(spaceGUID))
				Expect(serviceInstance.Share).To(Equal(share))
			})
		})
		Context("When the instance does not exist", func() {
			BeforeEach(func() {
				mock.ExpectQuery("SELECT service_instances.id FROM service_instances WHERE service_instance.id = ?").WithArgs(serviceID)
			})
			JustBeforeEach(func() {
				serviceInstance, err = sqlStore.RetrieveInstanceDetails(serviceID)
			})
			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(reflect.DeepEqual(serviceInstance,nfsbroker.ServiceInstance{})).To(BeTrue())
			})
		})

	})

	Describe("RetrieveBindingDetails", func() {
		var appGUID, planID, serviceID, bindingID string
		var bindResource brokerapi.BindResource
		var parameters map[string]interface{}
		var err error
		var bindDetails brokerapi.BindDetails

		db, mock, err := sqlmock.New()
		sqlStore := nfsbroker.SqlStore{Database:nfsbrokerfakes.FakeSQLMockConnection{db},
			StoreType:"mysql"}

		Context("When the instance exists", func() {
			BeforeEach(func() {
				Expect(err).NotTo(HaveOccurred())
				appGUID = "instance_123"
				planID = "plan_123"
				serviceID = "service_123"
				bindResource = brokerapi.BindResource{AppGuid: appGUID, Route: "binding-route"}

				columns := []string{"id", "value"}
				rows := sqlmock.NewRows(columns)
				jsonvalue, err := json.Marshal(brokerapi.BindDetails{AppGUID: appGUID, PlanID: planID, ServiceID: serviceID, BindResource: &bindResource, Parameters: parameters})
				Expect(err).NotTo(HaveOccurred())
				rows.AddRow(bindingID, jsonvalue)

				mock.ExpectQuery("SELECT service_bindings.id FROM service_bindings WHERE service_bindings.id = ?").WithArgs(serviceID).WillReturnRows(rows)
			})
			JustBeforeEach(func() {

				bindDetails, err = sqlStore.RetrieveBindingDetails(serviceID)
			})
			It("should return the binding details", func() {
				Expect(err).To(BeNil())
				Expect(mock.ExpectationsWereMet()).Should(Succeed())
				Expect(bindDetails.ServiceID).To(Equal(serviceID))
				Expect(bindDetails.PlanID).To(Equal(planID))
				Expect(bindDetails.AppGUID).To(Equal(appGUID))
				Expect(bindDetails.BindResource.AppGuid).To(Equal(appGUID))
				Expect(bindDetails.BindResource.Route).To(Equal("binding-route"))
				Expect(bindDetails.Parameters).To(Equal(parameters))
			})
		})
		Context("When the binding does not exist", func() {
			BeforeEach(func() {
				mock.ExpectQuery("SELECT service_bindings.id FROM service_bindings WHERE service_bindings.id = ?").WithArgs(serviceID)
			})
			JustBeforeEach(func() {
				bindDetails, err = sqlStore.RetrieveBindingDetails(serviceID)
			})
			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(reflect.DeepEqual(bindDetails, brokerapi.BindDetails{})).To(BeTrue())
			})
		})
	})
})
