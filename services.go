package main

import (
	"encoding/json"
	"io/ioutil"

	"github.com/pivotal-cf/brokerapi"
)

type Services interface {
	List() []brokerapi.Service
}

type services struct {
	services []brokerapi.Service
}

func NewServicesFromConfig(pathToServicesConfig string) (Services, error) {
	contents, err := ioutil.ReadFile(pathToServicesConfig)
	if err != nil {
		return nil, err
	}

	var s []brokerapi.Service
	err = json.Unmarshal(contents, &s)
	if err != nil {
		return nil, err
	}

	return &services{s}, nil
}

func (s *services) List() []brokerapi.Service {
	return s.services
}
