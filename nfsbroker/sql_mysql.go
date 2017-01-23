package nfsbroker

import (
	"fmt"

	"crypto/tls"
	"crypto/x509"

	"code.cloudfoundry.org/goshims/sqlshim"
	"code.cloudfoundry.org/lager"
	"github.com/go-sql-driver/mysql"
	"errors"
)

type mysqlConnection struct {
	sql                sqlshim.Sql
	dbConnectionString string
}

func NewMySql(logger lager.Logger, username, password, host, port, dbName, caCert string) SqlVariant {
	return NewMySqlWithSqlObject(logger, username, password, host, port, dbName, caCert, &sqlshim.SqlShim{})
}

func NewMySqlWithSqlObject(logger lager.Logger, username, password, host, port, dbName, caCert string, sql sqlshim.Sql) SqlVariant {
	logger = logger.Session("new-mysql-connection")
	logger.Info("start")
	defer logger.Info("end")

	var databaseConnectionString string
	switch caCert {
	case "":
		logger.Debug("insecure-mysql")
		databaseConnectionString = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", username, password, host, port, dbName)
	default:
		logger.Debug("secure-mysql")
		certBytes := []byte(caCert)

		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM(certBytes); !ok {
			logger.Fatal("failed-to-parse-sql-ca", fmt.Errorf("Invalid CA Cert for %s", dbName))
		}

		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
			RootCAs:            caCertPool,
		}

		mysql.RegisterTLSConfig("bbs-tls", tlsConfig)
		databaseConnectionString = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?tls=bbs-tls", username, password, host, port, dbName)
	}

	return &mysqlConnection{
		sql:                sql,
		dbConnectionString: databaseConnectionString,
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
