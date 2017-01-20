package nfsbroker_test

import (
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/nfsbroker/nfsbroker"

	"code.cloudfoundry.org/goshims/sqlshim/sql_fake"
	"code.cloudfoundry.org/nfsbroker/nfsbrokerfakes"
	"errors"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("SqlConnection", func() {
	var (
		database   nfsbroker.SqlConnection
		logger     lager.Logger
		toDatabase = &nfsbrokerfakes.FakeSqlVariant{}
		fakeSqlDb  = &sql_fake.FakeSqlDB{}
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test-SqlConnection")
		database = nfsbroker.NewSqlConnection(toDatabase)
	})

	Describe(".Connect", func() {
		var (
			err error
		)

		Context("when it can connect to a valid database", func() {
			BeforeEach(func() {
				toDatabase.ConnectReturns(fakeSqlDb, nil)
			})

			It("reports no error", func() {
				err = database.Connect(logger)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when it cannot connect to a valid database", func() {
			BeforeEach(func() {
				toDatabase.ConnectReturns(nil, errors.New("something wrong"))
			})

			It("reports error", func() {
				err = database.Connect(logger)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when it is give invalid database", func() {
			It("reports error", func() {
				invalid := nfsbroker.NewSqlConnection(nil)
				defer func() {
					r := recover()
					Expect(r).To(Equal("sqlConnection is an abstract class"))
				}()
				invalid.Connect(logger)
			})
		})
	})

	Context("when connected", func() {
		var query = `something else`

		BeforeEach(func() {
			toDatabase.ConnectReturns(fakeSqlDb, nil)
			database.Connect(logger)
		})

		Describe(".Ping", func() {
			It("should call through", func() {
				database.Ping()
				Expect(fakeSqlDb.PingCallCount()).To(Equal(1))
			})
		})
		Describe(".Close", func() {
			It("should call through", func() {
				database.Close()
				Expect(fakeSqlDb.CloseCallCount()).To(Equal(1))
			})
		})
		Describe(".SetMaxIdleConns", func() {
			It("should call through", func() {
				database.SetMaxIdleConns(1)
				Expect(fakeSqlDb.SetMaxIdleConnsCallCount()).To(Equal(1))
			})
		})
		Describe(".SetMaxOpenConns", func() {
			It("should call through", func() {
				database.SetMaxOpenConns(1)
				Expect(fakeSqlDb.SetMaxOpenConnsCallCount()).To(Equal(1))
			})
		})
		Describe(".SetConnMaxLifetime", func() {
			It("should call through", func() {
				database.SetConnMaxLifetime(time.Duration(1))
				Expect(fakeSqlDb.SetConnMaxLifetimeCallCount()).To(Equal(1))
			})
		})
		Describe(".Stats", func() {
			It("should call through", func() {
				database.Stats()
				Expect(fakeSqlDb.StatsCallCount()).To(Equal(1))
			})
		})
		Describe(".Prepare", func() {
			It("should call through", func() {
				database.Prepare("")
				Expect(fakeSqlDb.PrepareCallCount()).To(Equal(1))
			})
			It("should flavorify a query", func() {
				toDatabase.FlavorifyReturns(query)
				database.Prepare(`something`)
				Expect(fakeSqlDb.PrepareArgsForCall(fakeSqlDb.PrepareCallCount()-1)).To(Equal(query))
			})
		})
		Describe(".Exec", func() {
			It("should call through", func() {
				database.Exec("")
				Expect(fakeSqlDb.ExecCallCount()).To(Equal(1))
			})
			It("should flavorify a query", func() {
				toDatabase.FlavorifyReturns(query)
				database.Exec(`something`)
				Expect(fakeSqlDb.ExecArgsForCall(fakeSqlDb.ExecCallCount()-1)).To(Equal(query))
			})
		})
		Describe(".Query", func() {
			It("should call through", func() {
				database.Query("")
				Expect(fakeSqlDb.QueryCallCount()).To(Equal(1))
			})
			It("should flavorify a query", func() {
				toDatabase.FlavorifyReturns(query)
				database.Query(`something`)
				Expect(fakeSqlDb.QueryArgsForCall(fakeSqlDb.QueryCallCount()-1)).To(Equal(query))
			})
		})
		Describe(".QueryRow", func() {
			It("should call through", func() {
				database.QueryRow("")
				Expect(fakeSqlDb.QueryRowCallCount()).To(Equal(1))
			})
			It("should flavorify a query", func() {
				toDatabase.FlavorifyReturns(query)
				database.QueryRow(`something`)
				Expect(fakeSqlDb.QueryRowArgsForCall(fakeSqlDb.QueryRowCallCount()-1)).To(Equal(query))
			})
		})
		Describe(".Begin", func() {
			It("should call through", func() {
				database.Begin()
				Expect(fakeSqlDb.BeginCallCount()).To(Equal(1))
			})
		})
		Describe(".Driver", func() {
			It("should call through", func() {
				database.Driver()
				Expect(fakeSqlDb.DriverCallCount()).To(Equal(1))
			})
		})
	})
})
