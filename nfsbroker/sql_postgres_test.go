package nfsbroker_test

import (
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/nfsbroker/nfsbroker"

	"errors"

	"code.cloudfoundry.org/goshims/sqlshim/sql_fake"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("postgresConnection", func() {
	var (
		database nfsbroker.SqlVariant
		logger   lager.Logger
		fakeSql  = &sql_fake.FakeSql{}
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test-SqlConnection")
		database = nfsbroker.NewPostgresWithSqlObject("username", "password", "host", "port", "dbName", fakeSql)
	})

	Describe(".Connect", func() {
		var (
			err error
		)

		Context("when it can connect to a valid database", func() {
			BeforeEach(func() {
				fakeSql.OpenReturns(nil, nil)
			})

			It("reports no error", func() {
				_, err = database.Connect(logger)
				Expect(err).NotTo(HaveOccurred())
				dbType, connectionString := fakeSql.OpenArgsForCall(0)
				Expect(dbType).To(Equal("postgres"))
				Expect(connectionString).To(Equal("postgres://username:password@host:port/dbName?sslmode=disable"))
			})
		})

		Context("when it cannot connect to a valid database", func() {
			BeforeEach(func() {
				fakeSql.OpenReturns(nil, errors.New("something wrong"))
			})

			It("reports error", func() {
				_, err = database.Connect(logger)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe(".Flavorify", func() {
		It("should return unaltered query", func() {
			query := `INSERT INTO service_instances (id, value) VALUES (?, ?)`
			Expect(database.Flavorify(query)).To(Equal(`INSERT INTO service_instances (id, value) VALUES ($1, $2)`))
		})
	})
})
