package nfsbroker

import (
	"fmt"
	"strings"

	"code.cloudfoundry.org/goshims/sqlshim"
	"code.cloudfoundry.org/lager"
)

type postgresConnection struct {
	sql                sqlshim.Sql
	dbConnectionString string
}

func NewPostgres(username, password, host, port, dbName string) SqlVariant {
	return &postgresConnection{
		sql:                &sqlshim.SqlShim{},
		dbConnectionString: fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", username, password, host, port, dbName),
	}
}

func NewPostgresWithSqlObject(username, password, host, port, dbName string, sql sqlshim.Sql) SqlVariant {
	return &postgresConnection{
		sql:                sql,
		dbConnectionString: fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", username, password, host, port, dbName),
	}
}

func (c *postgresConnection) Connect(logger lager.Logger) (sqlshim.SqlDB, error) {
	logger = logger.Session("postgres-connection-connect")
	logger.Info("start")
	defer logger.Info("end")
	sqlDB, err := c.sql.Open("postgres", c.dbConnectionString)
	return sqlDB, err
}

func (c *postgresConnection) Flavorify(query string) string {
	strParts := strings.Split(query, "?")
	for i := 1; i < len(strParts); i++ {
		strParts[i-1] = fmt.Sprintf("%s$%d", strParts[i-1], i)
	}
	return strings.Join(strParts, "")
}
