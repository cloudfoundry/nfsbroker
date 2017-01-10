package nfsbroker

import (
	"encoding/json"
	"fmt"
	"os"

	"code.cloudfoundry.org/goshims/ioutilshim"
	"code.cloudfoundry.org/goshims/sqlshim"
	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"
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

func NewSqlStore(logger lager.Logger, sql sqlshim.Sql, dbDriver, username, password, host, port, schema string) (Store, error) {
	var dbConnectionString string
	if dbDriver == "mysql" {
		dbConnectionString = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", username, password, host, port, schema)
	} else if dbDriver == "postgres" {
		dbConnectionString = fmt.Sprintf("postgres://%s:%s@%s:%s/%s", username, password, host, port, schema)
	} else {
		err := fmt.Errorf("Unrecognized Driver: %s", dbDriver)
		logger.Error("db-driver-unrecognized", err)
		return nil, err
	}

	logger = logger.Session("new-sql-store")
	logger.Info("start")
	defer logger.Info("end")

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
	logger = logger.Session("restore-state")
	logger.Info("start")
	defer logger.Info("end")

	query := `SELECT id, value FROM service_instances`
	rows, err := s.sqlDB.Query(query)
	if err != nil {
		logger.Error("failed-query", err)
		return err
	}
	if rows != nil {
		for rows.Next() {
			var (
				id, value       string
				serviceInstance ServiceInstance
			)

			err := rows.Scan(
				&id,
				&value,
			)
			if err != nil {
				logger.Error("failed-scanning", err)
				continue
			}

			err = json.Unmarshal([]byte(value), &serviceInstance)
			if err != nil {
				logger.Error("failed-unmarshaling", err)
				continue
			}
			state.InstanceMap[id] = serviceInstance
		}

		if rows.Err() != nil {
			logger.Error("failed-getting-next-row", rows.Err())
		}
	}

	query = `SELECT id, value FROM service_bindings`
	_, err = s.sqlDB.Query(query)
	if err != nil {
		logger.Error("failed-query", err)
		return err
	}
	if rows != nil {
		for rows.Next() {
			var (
				id, value      string
				serviceBinding brokerapi.BindDetails
			)

			err := rows.Scan(
				&id,
				&value,
			)
			if err != nil {
				logger.Error("failed-scanning", err)
				continue
			}

			err = json.Unmarshal([]byte(value), &serviceBinding)
			if err != nil {
				logger.Error("failed-unmarshaling", err)
				continue
			}
			state.BindingMap[id] = serviceBinding
		}

		if rows.Err() != nil {
			logger.Error("failed-getting-next-row", rows.Err())
		}
	}

	return nil
}

func (s *SqlStore) Save(logger lager.Logger, state *DynamicState, instanceId, bindingId string) error {
	logger = logger.Session("save-state")
	logger.Info("start")
	defer logger.Info("end")

	if instanceId != "" {
		instance, ok := state.InstanceMap[instanceId]
		if ok {
			jsonValue, err := json.Marshal(&instance)
			if err != nil {
				logger.Error("failed-marshaling", err)
				return err
			}

			// todo--what if the row already exists?
			query := `INSERT INTO service_instances (id, value) VALUES (?, ?)`

			_, err = s.sqlDB.Exec(query, instanceId, jsonValue)
			if err != nil {
				logger.Error("failed-exec", err)
				return err
			}
		} else {
			query := `DELETE FROM service_instances WHERE id=?`
			_, err := s.sqlDB.Exec(query, instanceId)
			if err != nil {
				logger.Error("failed-exec", err)
				return err
			}
		}
	}

	if bindingId != "" {
		binding, ok := state.BindingMap[bindingId]
		if ok {
			jsonValue, err := json.Marshal(&binding)
			if err != nil {
				logger.Error("failed-marshaling", err)
				return err
			}

			// todo--what if the row already exists?
			query := `INSERT INTO service_bindings (id, value) VALUES (?, ?)`

			_, err = s.sqlDB.Exec(query, bindingId, jsonValue)
			if err != nil {
				logger.Error("failed-exec", err)
				return err
			}
		} else {
			query := `DELETE FROM service_bindings WHERE id=?`
			_, err := s.sqlDB.Exec(query, bindingId)
			if err != nil {
				logger.Error("failed-exec", err)
				return err
			}
		}
	}

	return nil
}

func (s *SqlStore) Cleanup() error {
	return s.sqlDB.Close()
}
