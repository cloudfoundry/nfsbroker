package nfsbroker

import (
	"fmt"

	"code.cloudfoundry.org/goshims/sqlshim"
	"code.cloudfoundry.org/lager"
)

type mysqlConnection struct {
	sql                sqlshim.Sql
	dbConnectionString string
}

func NewMySql(username, password, host, port, dbName string) SqlVariant {
	return &mysqlConnection{
		sql:                &sqlshim.SqlShim{},
		dbConnectionString: fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", username, password, host, port, dbName),
	}
}

func NewMySqlWithSqlObject(username, password, host, port, dbName string, sql sqlshim.Sql) SqlVariant {
	return &mysqlConnection{
		sql:                sql,
		dbConnectionString: fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", username, password, host, port, dbName),
	}
}

func (c *mysqlConnection) Connect(logger lager.Logger) (sqlshim.SqlDB, error) {
	logger = logger.Session("mysql-connection-connect")
	logger.Info("start")
	defer logger.Info("end")
	sqlDB, err := c.sql.Open("mysql", c.dbConnectionString)
	return sqlDB, err
}

func (c *mysqlConnection) Flavorify(query string) string {
	return query
}
