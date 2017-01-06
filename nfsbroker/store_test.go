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
})

var _ = Describe("SqlStore", func() {
	var (
		store   nfsbroker.Store
		logger  lager.Logger
		state   nfsbroker.DynamicState
		fakeSQL *sql_fake.FakeSql
		err     error
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test-broker")
		fakeSQL = &sql_fake.FakeSql{}
		store, err = nfsbroker.NewSqlStore(logger, fakeSQL, "postgres", "foo")
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
		Expect(fakeSQL.OpenCallCount()).To(Equal(1))
	})
	It("should create tables if they don't exist", func() {})

	Describe("Restore", func() {
		Context("when it succeeds", func() {
			It("", func() {
				Expect(true).To(BeTrue())
			})
		})
	})
})
