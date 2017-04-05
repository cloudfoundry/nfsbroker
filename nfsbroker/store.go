package nfsbroker

import (
	"code.cloudfoundry.org/goshims/ioutilshim"
	"code.cloudfoundry.org/lager"
	"encoding/json"
	"github.com/pivotal-cf/brokerapi"
	"golang.org/x/crypto/bcrypt"
	"reflect"
)

//go:generate counterfeiter -o ../nfsbrokerfakes/fake_store.go . Store
type Store interface {
	RetrieveInstanceDetails(id string) (ServiceInstance, error)
	RetrieveBindingDetails(id string) (brokerapi.BindDetails, error)

	CreateInstanceDetails(id string, details ServiceInstance) error
	CreateBindingDetails(id string, details brokerapi.BindDetails) error

	DeleteInstanceDetails(id string) error
	DeleteBindingDetails(id string) error

	IsInstanceConflict(id string, details ServiceInstance) bool
	IsBindingConflict(id string, details brokerapi.BindDetails) bool

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

// Utility methods for storing bindings with secrets stripped out
const HashKey = "paramsHash"

func redactBindingDetails(details brokerapi.BindDetails) (brokerapi.BindDetails, error) {
	if details.Parameters == nil {
		return details, nil
	}
	if len(details.Parameters) == 1 {
		if _, ok := details.Parameters[HashKey]; ok {
			return details, nil
		}
	}

	s, err := json.Marshal(details.Parameters)
	if err != nil {
		return brokerapi.BindDetails{}, err
	}
	s, err = bcrypt.GenerateFromPassword(s, bcrypt.DefaultCost)
	if err != nil {
		return brokerapi.BindDetails{}, err
	}
	details.Parameters = map[string]interface{}{HashKey: string(s)}
	return details, nil
}

func isBindingConflict(s Store, id string, details brokerapi.BindDetails) bool {
	if existing, err := s.RetrieveBindingDetails(id); err == nil {
		if existing.AppGUID != details.AppGUID {
			return true
		}
		if existing.PlanID != details.PlanID {
			return true
		}
		if existing.ServiceID != details.ServiceID {
			return true
		}
		if !reflect.DeepEqual(details.BindResource, existing.BindResource) {
			return true
		}
		if (details.Parameters == nil) && (existing.Parameters == nil) {
			return false
		}
		if (details.Parameters == nil) || (existing.Parameters == nil) {
			return true
		}

		s, err := json.Marshal(details.Parameters)
		if err != nil {
			return true
		}
		h, _ := existing.Parameters[HashKey]
		if bcrypt.CompareHashAndPassword([]byte(h.(string)), s) != nil {
			return true
		}
	}
	return false
}
