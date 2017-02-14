package nfsbroker

import (
	"code.cloudfoundry.org/goshims/ioutilshim"
	"code.cloudfoundry.org/lager"
)

const SQLSTORE = "SQL_Store"
const FILESTORE = "File_Store"

//go:generate counterfeiter -o ../nfsbrokerfakes/fake_store.go . Store
type Store interface {
	GetType() string
	Restore(logger lager.Logger, state *DynamicState) error
	Save(logger lager.Logger, state *DynamicState, instanceId, bindingId string) error
	Cleanup() error

}

func NewStore(logger lager.Logger, dbDriver, dbUsername, dbPassword, dbHostname, dbPort, dbName, dbCACert, fileName string) Store {
	if dbDriver != "" {
		store, err := NewSqlStore(logger, dbDriver, dbUsername, dbPassword, dbHostname, dbPort, dbName, dbCACert)
		if err != nil {
			logger.Fatal("failed-creating-sql-store", err)
		}
		return store
	} else {
		return NewFileStore(fileName, &ioutilshim.IoutilShim{})
	}
}
