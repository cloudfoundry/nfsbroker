package nfsbroker

import (
	"encoding/json"
	"fmt"
	"os"

	"code.cloudfoundry.org/goshims/ioutilshim"
	"code.cloudfoundry.org/goshims/sqlshim"
	"code.cloudfoundry.org/lager"
)

//go:generate counterfeiter -o ../nfsbrokerfakes/fake_store.go . Store
type Store interface {
	Restore(logger lager.Logger, state *DynamicState) error
	Save(logger lager.Logger, state *DynamicState, instanceId, bindingId string) error
	Cleanup() error
}

type FileStore struct {
	fileName string
	ioutil   ioutilshim.Ioutil
}

func NewFileStore(
	fileName string,
	ioutil ioutilshim.Ioutil,
) Store {
	return &FileStore{
		fileName: fileName,
		ioutil:   ioutil,
	}
}

type SqlStore struct {
	sql   sqlshim.Sql
	sqlDB sqlshim.SqlDB
}

func (s *FileStore) Restore(logger lager.Logger, state *DynamicState) error {
	logger = logger.Session("restore-state")
	logger.Info("start")
	defer logger.Info("end")

	serviceData, err := s.ioutil.ReadFile(s.fileName)
	if err != nil {
		logger.Error(fmt.Sprintf("failed-to-read-state-file: %s", s.fileName), err)
		return err
	}

	err = json.Unmarshal(serviceData, state)
	if err != nil {
		logger.Error(fmt.Sprintf("failed-to-unmarshall-state from state-file: %s", s.fileName), err)
		return err
	}
	logger.Info("state-restored", lager.Data{"state-file": s.fileName})

	return err
}

func (s *FileStore) Save(logger lager.Logger, state *DynamicState, _, _ string) error {
	logger = logger.Session("serialize-state")
	logger.Info("start")
	defer logger.Info("end")

	stateData, err := json.Marshal(state)
	if err != nil {
		logger.Error("failed-to-marshall-state", err)
		return err
	}

	err = s.ioutil.WriteFile(s.fileName, stateData, os.ModePerm)
	if err != nil {
		logger.Error(fmt.Sprintf("failed-to-write-state-file: %s", s.fileName), err)
		return err
	}

	logger.Info("state-saved", lager.Data{"state-file": s.fileName})

	return nil
}

func (s *FileStore) Cleanup() error {
	return nil
}

func NewSqlStore(logger lager.Logger, sql sqlshim.Sql, dbDriver string, dbConnectionString string) (Store, error) {
	sqlDB, err := sql.Open(dbDriver, dbConnectionString)
	if err != nil {
		logger.Error("failed-to-open-sql", err)
		return nil, err
	}

	err = sqlDB.Ping()
	if err != nil {
		logger.Error("sql-failed-to-connect", err)
		return nil, err
	}

	// TODO: uniqueify table names?
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS service_instances(
			id VARCHAR(255) PRIMARY KEY,
			value VARCHAR(4096)
		)
	`)
	if err != nil {
		return nil, err
	}
	_, err = sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS service_bindings(
			id VARCHAR(255) PRIMARY KEY,
			value VARCHAR(4096)
		)
	`)
	if err != nil {
		return nil, err
	}

	return &SqlStore{
		sql:   sql,
		sqlDB: sqlDB,
	}, nil
}

func (s *SqlStore) Restore(logger lager.Logger, state *DynamicState) error {
	return nil
}

func (s *SqlStore) Save(logger lager.Logger, state *DynamicState, instanceId, bindingId string) error {
	return nil
}

func (s *SqlStore) Cleanup() error {
	return s.sqlDB.Close()
}
