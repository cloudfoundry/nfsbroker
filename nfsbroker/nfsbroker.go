package nfsbroker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"reflect"
	"sync"

	"crypto/md5"

	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/goshims/osshim"
	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"
)

const (
	PermissionVolumeMount = brokerapi.RequiredPermission("volume_mount")
	DefaultContainerPath  = "/var/vcap/data"
)

const (
	Username string = "kerberosPrincipal"
	Secret   string = "kerberosKeytab"
)

type staticState struct {
	ServiceName string `json:"ServiceName"`
	ServiceId   string `json:"ServiceId"`
}

type ServiceInstance struct {
	ServiceID        string `json:"service_id"`
	PlanID           string `json:"plan_id"`
	OrganizationGUID string `json:"organization_guid"`
	SpaceGUID        string `json:"space_guid"`
	Share            string
}

type DynamicState struct {
	InstanceMap map[string]ServiceInstance
	BindingMap  map[string]brokerapi.BindDetails
}

type lock interface {
	Lock()
	Unlock()
}

type Broker struct {
	logger  lager.Logger
	dataDir string
	os      osshim.Os
	mutex   lock
	clock   clock.Clock
	static  staticState
	dynamic DynamicState
	store   Store
}

func New(
	logger lager.Logger,
	serviceName, serviceId, dataDir string,
	os osshim.Os,
	clock clock.Clock,
	store Store,
) *Broker {

	theBroker := Broker{
		logger:  logger,
		dataDir: dataDir,
		os:      os,
		mutex:   &sync.Mutex{},
		clock:   clock,
		store:   store,
		static: staticState{
			ServiceName: serviceName,
			ServiceId:   serviceId,
		},
		dynamic: DynamicState{
			InstanceMap: map[string]ServiceInstance{},
			BindingMap:  map[string]brokerapi.BindDetails{},
		},
	}

	theBroker.store.Restore(logger, &theBroker.dynamic)

	return &theBroker
}

func (b *Broker) Services(_ context.Context) []brokerapi.Service {
	logger := b.logger.Session("services")
	logger.Info("start")
	defer logger.Info("end")

	return []brokerapi.Service{{
		ID:            b.static.ServiceId,
		Name:          b.static.ServiceName,
		Description:   "Existing NFSv3 volumes (see: https://code.cloudfoundry.org/nfs-volume-release/)",
		Bindable:      true,
		PlanUpdatable: false,
		Tags:          []string{"nfs"},
		Requires:      []brokerapi.RequiredPermission{PermissionVolumeMount},

		Plans: []brokerapi.ServicePlan{
			{
				Name:        "Existing",
				ID:          "Existing",
				Description: "A preexisting filesystem",
			},
		},
	}}
}

func (b *Broker) Provision(context context.Context, instanceID string, details brokerapi.ProvisionDetails, asyncAllowed bool) (brokerapi.ProvisionedServiceSpec, error) {
	logger := b.logger.Session("provision").WithData(lager.Data{"instanceID": instanceID})
	logger.Info("start")
	defer logger.Info("end")

	if b.instanceConflicts(details, instanceID) {
		return brokerapi.ProvisionedServiceSpec{}, brokerapi.ErrInstanceAlreadyExists
	}

	type Configuration struct {
		Share string `json:"share"`
	}
	var configuration Configuration

	var decoder *json.Decoder = json.NewDecoder(bytes.NewBuffer(details.RawParameters))
	err := decoder.Decode(&configuration)
	if err != nil {
		return brokerapi.ProvisionedServiceSpec{}, brokerapi.ErrRawParamsInvalid
	}

	if configuration.Share == "" {
		return brokerapi.ProvisionedServiceSpec{}, errors.New("config requires a \"share\" key")
	}

	b.dynamic.InstanceMap[instanceID] = ServiceInstance{
		details.ServiceID,
		details.PlanID,
		details.OrganizationGUID,
		details.SpaceGUID,
		configuration.Share}

	b.mutex.Lock()
	defer b.mutex.Unlock()

	defer b.store.Save(logger, &b.dynamic, instanceID, "")

	return brokerapi.ProvisionedServiceSpec{IsAsync: false}, nil
}

func (b *Broker) Deprovision(context context.Context, instanceID string, details brokerapi.DeprovisionDetails, asyncAllowed bool) (brokerapi.DeprovisionServiceSpec, error) {
	logger := b.logger.Session("deprovision")
	logger.Info("start")
	defer logger.Info("end")

	b.mutex.Lock()
	defer b.mutex.Unlock()

	_, instanceExists := b.dynamic.InstanceMap[instanceID]
	if !instanceExists {
		return brokerapi.DeprovisionServiceSpec{}, brokerapi.ErrInstanceDoesNotExist
	} else {
		delete(b.dynamic.InstanceMap, instanceID)
		b.store.Save(logger, &b.dynamic, instanceID, "")
	}

	return brokerapi.DeprovisionServiceSpec{IsAsync: false, OperationData: "deprovision"}, nil
}

func (b *Broker) Bind(context context.Context, instanceID string, bindingID string, details brokerapi.BindDetails) (brokerapi.Binding, error) {
	logger := b.logger.Session("bind")
	logger.Info("start", lager.Data{"bindingID": bindingID, "details": details})
	defer logger.Info("end")

	b.mutex.Lock()
	defer b.mutex.Unlock()

	defer b.store.Save(logger, &b.dynamic, "", bindingID)

	logger.Info("Starting nfsbroker bind")
	instanceDetails, ok := b.dynamic.InstanceMap[instanceID]
	if !ok {
		return brokerapi.Binding{}, brokerapi.ErrInstanceDoesNotExist
	}

	if details.AppGUID == "" {
		return brokerapi.Binding{}, brokerapi.ErrAppGuidNotProvided
	}

	mode, err := evaluateMode(details.Parameters)
	if err != nil {
		return brokerapi.Binding{}, err
	}

	if b.bindingConflicts(bindingID, details) {
		return brokerapi.Binding{}, brokerapi.ErrBindingAlreadyExists
	}

	b.dynamic.BindingMap[bindingID] = details

	var uid interface{}
	var exist bool
	if uid, exist = details.Parameters["uid"]; !exist {
		return brokerapi.Binding{}, errors.New("config requires a \"uid\"")
	}

	var gid interface{}
	if gid, exist = details.Parameters["gid"]; !exist {
		return brokerapi.Binding{}, errors.New("config requires a \"gid\"")
	}

	mountConfig := map[string]interface{}{"source": fmt.Sprintf("nfs://%s?uid=%s&gid=%s", instanceDetails.Share, uid.(string), gid.(string))}

	s, err := b.hash(mountConfig)
	if err != nil {
		logger.Error("error-calculating-volume-id", err, lager.Data{"config": mountConfig, "bindingID": bindingID, "instanceID": instanceID})
		return brokerapi.Binding{}, err
	}
	volumeId := fmt.Sprintf("%s-%s", instanceID, s)

	return brokerapi.Binding{
		Credentials: struct{}{}, // if nil, cloud controller chokes on response
		VolumeMounts: []brokerapi.VolumeMount{{
			ContainerDir: evaluateContainerPath(details.Parameters, instanceID),
			Mode:         mode,
			Driver:       "nfsv3driver",
			DeviceType:   "shared",
			Device: brokerapi.SharedDevice{
				VolumeId:    volumeId,
				MountConfig: mountConfig,
			},
		}},
	}, nil
}

func (b *Broker) hash(mountConfig map[string]interface{}) (string, error) {
	var (
		bytes []byte
		err   error
	)
	if bytes, err = json.Marshal(mountConfig); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", md5.Sum(bytes)), nil
}

func (b *Broker) Unbind(context context.Context, instanceID string, bindingID string, details brokerapi.UnbindDetails) error {
	logger := b.logger.Session("unbind")
	logger.Info("start")
	defer logger.Info("end")

	b.mutex.Lock()
	defer b.mutex.Unlock()

	defer b.store.Save(logger, &b.dynamic, "", bindingID)

	if _, ok := b.dynamic.InstanceMap[instanceID]; !ok {
		return brokerapi.ErrInstanceDoesNotExist
	}

	if _, ok := b.dynamic.BindingMap[bindingID]; !ok {
		return brokerapi.ErrBindingDoesNotExist
	}

	delete(b.dynamic.BindingMap, bindingID)

	return nil
}

func (b *Broker) Update(context context.Context, instanceID string, details brokerapi.UpdateDetails, asyncAllowed bool) (brokerapi.UpdateServiceSpec, error) {
	panic("not implemented")
}

func (b *Broker) LastOperation(_ context.Context, instanceID string, operationData string) (brokerapi.LastOperation, error) {
	logger := b.logger.Session("last-operation").WithData(lager.Data{"instanceID": instanceID})
	logger.Info("start")
	defer logger.Info("end")

	b.mutex.Lock()
	defer b.mutex.Unlock()

	switch operationData {
	default:
		return brokerapi.LastOperation{}, errors.New("unrecognized operationData")
	}
}

func (b *Broker) instanceConflicts(details brokerapi.ProvisionDetails, instanceID string) bool {
	if existing, ok := b.dynamic.InstanceMap[instanceID]; ok {
		if !reflect.DeepEqual(details, existing) {
			return true
		}
	}
	return false
}

func (b *Broker) bindingConflicts(bindingID string, details brokerapi.BindDetails) bool {
	if existing, ok := b.dynamic.BindingMap[bindingID]; ok {
		if !reflect.DeepEqual(details, existing) {
			return true
		}
	}
	return false
}

func evaluateContainerPath(parameters map[string]interface{}, volId string) string {
	if containerPath, ok := parameters["mount"]; ok && containerPath != "" {
		return containerPath.(string)
	}

	return path.Join(DefaultContainerPath, volId)
}

func evaluateMode(parameters map[string]interface{}) (string, error) {
	if ro, ok := parameters["readonly"]; ok {
		switch ro := ro.(type) {
		case bool:
			return readOnlyToMode(ro), nil
		default:
			return "", brokerapi.ErrRawParamsInvalid
		}
	}
	return "rw", nil
}

func readOnlyToMode(ro bool) string {
	if ro {
		return "r"
	}
	return "rw"
}
