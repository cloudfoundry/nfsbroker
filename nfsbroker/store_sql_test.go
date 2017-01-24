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
)

var _ = Describe("SqlStore", func() {
	var (
		store       nfsbroker.Store
		logger      lager.Logger
		state       nfsbroker.DynamicState
		fakeSqlDb   = &sql_fake.FakeSqlDB{}
		fakeVariant = &nfsbrokerfakes.FakeSqlVariant{}
		err         error
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test-broker")
		fakeVariant.ConnectReturns(fakeSqlDb, nil)
		fakeVariant.FlavorifyStub = func(query string) string { return query }
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
			store.Restore(logger, &state)
		})

		Context("when it succeeds", func() {
			It("queries the database", func() {
				Expect(fakeSqlDb.QueryCallCount()).To(BeNumerically(">=", 2))
			})
		})
	})

	Describe("Save", func() {
		Context("when the row is added", func() {
			BeforeEach(func() {
				store.Save(logger, &state, "service-name", "")
			})
			It("is inserted", func() {
				Expect(fakeSqlDb.ExecCallCount()).To(BeNumerically(">=", 3))
				query, _ := fakeSqlDb.ExecArgsForCall(fakeSqlDb.ExecCallCount() - 1)
				Expect(query).To(ContainSubstring("INSERT INTO service_instances (id, value) VALUES"))
			})
		})
		Context("when the row is removed", func() {
			BeforeEach(func() {
				store.Save(logger, &state, "non-existent-service-name", "")
			})
			It("is deleted", func() {
				Expect(fakeSqlDb.ExecCallCount()).To(BeNumerically(">=", 3))
				query, _ := fakeSqlDb.ExecArgsForCall(fakeSqlDb.ExecCallCount() - 1)
				Expect(query).To(ContainSubstring("DELETE FROM service_instances WHERE id="))
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
			It("closes the db connection", func() {
				Expect(fakeSqlDb.CloseCallCount()).To(BeNumerically(">=", 1))
			})
		})
	})
})
