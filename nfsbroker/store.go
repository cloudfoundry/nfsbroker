package nfsbroker

import (
	"encoding/json"
	"fmt"
	"os"

	"code.cloudfoundry.org/goshims/ioutilshim"
	"code.cloudfoundry.org/goshims/sqlshim"
	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"
	"strings"
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

type DBType int
const (
	MySQL DBType = iota
	Postgres
)

type SqlStore struct {
	sql   sqlshim.Sql
	sqlDB sqlshim.SqlDB
	flavor DBType
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

func NewSqlStore(logger lager.Logger, sql sqlshim.Sql, dbDriver, username, password, host, port, dbName string) (Store, error) {
	var flavor DBType
	var dbConnectionString string
	if dbDriver == "mysql" {
		flavor = MySQL
		dbConnectionString = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", username, password, host, port, dbName)
	} else if dbDriver == "postgres" {
		// TODO handle optional SSL
		flavor = Postgres
		dbConnectionString = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", username, password, host, port, dbName)
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
		flavor: flavor,
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
	logger.Info("start", lager.Data{"instanceId": instanceId, "bindingId": bindingId})
	defer logger.Info("end")

	if instanceId != "" {
		instance, ok := state.InstanceMap[instanceId]
		if ok {
			logger.Info("instance-found", lager.Data{"instance": instance})
			jsonValue, err := json.Marshal(&instance)
			if err != nil {
				logger.Error("failed-marshaling", err)
				return err
			}

			// todo--what if the row already exists?

			query := Flavorify(`INSERT INTO service_instances (id, value) VALUES (?, ?)`, s.flavor)

			_, err = s.sqlDB.Exec(query, instanceId, jsonValue)
			if err != nil {
				logger.Error("failed-exec", err)
				return err
			}
		} else {
			query := Flavorify(`DELETE FROM service_instances WHERE id=?`, s.flavor)
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
			query := Flavorify(`INSERT INTO service_bindings (id, value) VALUES (?, ?)`, s.flavor)
			_, err = s.sqlDB.Exec(query, bindingId, jsonValue)
			if err != nil {
				logger.Error("failed-exec", err)
				return err
			}
		} else {
			query := Flavorify(`DELETE FROM service_bindings WHERE id=?`, s.flavor)
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

func Flavorify(query string, flavor DBType) string {
	if flavor == MySQL {
		return query
	}
	if flavor != Postgres {
		panic(fmt.Sprintf("Unrecognized DB flavor '%s'", flavor))
	}
	strParts := strings.Split(query, "?")
	for i := 1; i < len(strParts); i++ {
		strParts[i-1] = fmt.Sprintf("%s$%d", strParts[i-1], i)
	}
	return strings.Join(strParts, "")
}