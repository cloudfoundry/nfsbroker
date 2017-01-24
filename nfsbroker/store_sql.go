package nfsbroker

import (
	"fmt"

	"encoding/json"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"
)

type sqlStore struct {
	database SqlConnection
}

func NewSqlStore(logger lager.Logger, dbDriver, username, password, host, port, dbName, caCert string) (Store, error) {

	var err error
	var toDatabase SqlVariant
	switch dbDriver {
	case "mysql":
		toDatabase = NewMySqlVariant(username, password, host, port, dbName, caCert)
	case "postgres":
		toDatabase = NewPostgresVariant(username, password, host, port, dbName, caCert)
	default:
		err = fmt.Errorf("Unrecognized Driver: %s", dbDriver)
		logger.Error("db-driver-unrecognized", err)
		return nil, err
	}
	return NewSqlStoreWithVariant(logger, toDatabase)
}

func NewSqlStoreWithVariant(logger lager.Logger, toDatabase SqlVariant) (Store, error) {
	database := NewSqlConnection(toDatabase)

	err := initialize(logger, database)
	if err != nil {
		logger.Error("sql-failed-to-initialize-database", err)
		return nil, err
	}

	return &sqlStore{
		database: database,
	}, nil
}

func initialize(logger lager.Logger, db SqlConnection) error {
	logger = logger.Session("initialize-database")
	logger.Info("start")
	defer logger.Info("end")

	var err error
	err = db.Connect(logger)
	if err != nil {
		logger.Error("sql-failed-to-connect", err)
		return err
	}

	// TODO: uniquify table names?
	_, err = db.Exec(`
			CREATE TABLE IF NOT EXISTS service_instances(
				id VARCHAR(255) PRIMARY KEY,
				value VARCHAR(4096)
			)
		`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
			CREATE TABLE IF NOT EXISTS service_bindings(
				id VARCHAR(255) PRIMARY KEY,
				value VARCHAR(4096)
			)
		`)
	return err
}

func (s *sqlStore) Restore(logger lager.Logger, state *DynamicState) error {
	logger = logger.Session("restore-state")
	logger.Info("start")
	defer logger.Info("end")

	query := `SELECT id, value FROM service_instances`
	rows, err := s.database.Query(query)
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
	_, err = s.database.Query(query)
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

func (s *sqlStore) Save(logger lager.Logger, state *DynamicState, instanceId, bindingId string) error {
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

			query := `INSERT INTO service_instances (id, value) VALUES (?, ?)`

			_, err = s.database.Exec(query, instanceId, jsonValue)
			if err != nil {
				logger.Error("failed-exec", err)
				return err
			}
		} else {
			query := `DELETE FROM service_instances WHERE id=?`
			_, err := s.database.Exec(query, instanceId)
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
			_, err = s.database.Exec(query, bindingId, jsonValue)
			if err != nil {
				logger.Error("failed-exec", err)
				return err
			}
		} else {
			query := `DELETE FROM service_bindings WHERE id=?`
			_, err := s.database.Exec(query, bindingId)
			if err != nil {
				logger.Error("failed-exec", err)
				return err
			}
		}
	}

	return nil
}

func (s *sqlStore) Cleanup() error {
	return s.database.Close()
}
