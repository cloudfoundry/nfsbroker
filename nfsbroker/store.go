package nfsbroker

import (
	"code.cloudfoundry.org/goshims/ioutilshim"
	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"
)

//go:generate counterfeiter -o ../nfsbrokerfakes/fake_store.go . Store
type Store interface {
	RetrieveInstanceDetails(id string) (ServiceInstance, error)
	RetrieveBindingDetails(id string) (brokerapi.BindDetails, error)

	CreateInstanceDetails(id string, details ServiceInstance) error
	CreateBindingDetails(id string, details brokerapi.BindDetails) error

	DeleteInstanceDetails(id string) error
	DeleteBindingDetails(id string) error


	Restore(logger lager.Logger) error
	Save(logger lager.Logger) error
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
