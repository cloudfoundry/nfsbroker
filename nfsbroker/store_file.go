package nfsbroker

import (
	"code.cloudfoundry.org/goshims/ioutilshim"
	"code.cloudfoundry.org/lager"
	"encoding/json"
	"fmt"
	"os"
)

type fileStore struct {
	fileName string
	ioutil   ioutilshim.Ioutil
}

func NewFileStore(
	fileName string,
	ioutil ioutilshim.Ioutil,
) Store {
	return &fileStore{
		fileName: fileName,
		ioutil:   ioutil,
	}
}

func (s *fileStore) Restore(logger lager.Logger, state *DynamicState) error {
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

func (s *fileStore) Save(logger lager.Logger, state *DynamicState, _, _ string) error {
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

func (s *fileStore) Cleanup() error {
	return nil
}
