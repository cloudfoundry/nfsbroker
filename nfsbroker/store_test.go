package nfsbroker_test

import (
	"errors"

	"code.cloudfoundry.org/goshims/ioutilshim/ioutil_fake"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/nfsbroker/nfsbroker"
	"github.com/pivotal-cf/brokerapi"

	"code.cloudfoundry.org/goshims/sqlshim/sql_fake"
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
				err = store.Restore(logger, &state)
			})

			It("reads the file", func() {
				Expect(fakeIoutil.ReadFileCallCount()).To(Equal(1))
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when the file system is failing", func() {
			BeforeEach(func() {
				fakeIoutil.ReadFileReturns(nil, errors.New("badness"))
				err = store.Restore(logger, &state)
			})

			It("returns an error", func() {
				Expect(err).To(MatchError("badness"))
			})
		})
		Context("when there is junk in the file", func() {
			BeforeEach(func() {
				filecontents := "{serviceName: [some invalid state]}"
				fakeIoutil.ReadFileReturns([]byte(filecontents), nil)
				err = store.Restore(logger, &state)
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
				err = store.Save(logger, &state, "", "")
			})

			It("writes the file", func() {
				Expect(fakeIoutil.WriteFileCallCount()).To(Equal(1))
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when the file system is failing", func() {
			BeforeEach(func() {
				fakeIoutil.WriteFileReturns(errors.New("badness"))
				err = store.Save(logger, &state, "", "")
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

})

var _ = Describe("SqlStore", func() {
	var (
		store     nfsbroker.Store
		logger    lager.Logger
		state     nfsbroker.DynamicState
		fakeSql   *sql_fake.FakeSql
		fakeSqlDb *sql_fake.FakeSqlDB
		err       error
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test-broker")
		fakeSql = &sql_fake.FakeSql{}
		fakeSqlDb = &sql_fake.FakeSqlDB{}
		fakeSql.OpenReturns(fakeSqlDb, nil)
		store, err = nfsbroker.NewSqlStore(logger, fakeSql, "postgres", "foo", "foo", "foo", "foo", "foo")
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
		Expect(fakeSql.OpenCallCount()).To(Equal(1))
	})

	It("should ping the connection to make sure it works", func() {
		Expect(fakeSqlDb.PingCallCount()).To(Equal(1))
	})

	It("should create tables if they don't exist", func() {
		Expect(fakeSqlDb.ExecCallCount()).To(Equal(2))
		Expect(fakeSqlDb.ExecArgsForCall(0)).To(ContainSubstring("CREATE TABLE IF NOT EXISTS service_instances"))
		Expect(fakeSqlDb.ExecArgsForCall(1)).To(ContainSubstring("CREATE TABLE IF NOT EXISTS service_bindings"))
	})

	Describe("Restore", func() {
		BeforeEach(func() {
			store.Restore(logger, &state)
		})

		Context("when it succeeds", func() {
			It("", func() {
				Expect(fakeSqlDb.QueryCallCount()).To(Equal(2))
			})
		})
	})

	Describe("Save", func() {
		Context("when the row is added", func() {
			BeforeEach(func() {
				store.Save(logger, &state, "service-name", "")
			})
			It("", func() {
				Expect(fakeSqlDb.ExecCallCount()).To(Equal(3))
				query, _ := fakeSqlDb.ExecArgsForCall(2)
				Expect(query).To(ContainSubstring("INSERT INTO service_instances (id, value) VALUES (?, ?)"))
			})
		})
		Context("when the row is removed", func() {
			BeforeEach(func() {
				store.Save(logger, &state, "non-existent-service-name", "")
			})
			It("", func() {
				Expect(fakeSqlDb.ExecCallCount()).To(Equal(3))
				query, _ := fakeSqlDb.ExecArgsForCall(2)
				Expect(query).To(ContainSubstring("DELETE FROM service_instances WHERE id=?"))
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
				Expect(fakeSqlDb.CloseCallCount()).To(Equal(1))
			})
		})
	})
})
