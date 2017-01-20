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

var _ = Describe("mysqlConnection", func() {
	var (
		database nfsbroker.SqlVariant
		logger   lager.Logger
		fakeSql  = &sql_fake.FakeSql{}
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test-SqlConnection")
		database = nfsbroker.NewMySqlWithSqlObject("username", "password", "host", "port", "dbName", fakeSql)
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
			query := "query"
			Expect(database.Flavorify(query)).To(Equal(query))
		})
	})
})
