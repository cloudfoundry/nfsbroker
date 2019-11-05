package existingvolumebroker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"sync"

	"crypto/md5"

	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/goshims/osshim"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/service-broker-store/brokerstore"
	vmo "code.cloudfoundry.org/volume-mount-options"
	vmou "code.cloudfoundry.org/volume-mount-options/utils"
	"github.com/pivotal-cf/brokerapi"
)

const (
	DEFAULT_CONTAINER_PATH = "/var/vcap/data"
	SHARE_KEY              = "share"
	SOURCE_KEY             = "source"
	VERSION_KEY            = "version"
)

type lock interface {
	Lock()
	Unlock()
}

type BrokerType int

const (
	BrokerTypeNFS BrokerType = iota
	BrokerTypeSMB
)

type Broker struct {
	brokerType              BrokerType
	logger                  lager.Logger
	os                      osshim.Os
	mutex                   lock
	clock                   clock.Clock
	store                   brokerstore.Store
	services                Services
	configMask              vmo.MountOptsMask
	DisallowedBindOverrides []string
}

//go:generate counterfeiter -o fakes/fake_services.go . Services
type Services interface {
	List() []brokerapi.Service
}

func New(
	brokerType BrokerType,
	logger lager.Logger,
	services Services,
	os osshim.Os,
	clock clock.Clock,
	store brokerstore.Store,
	configMask vmo.MountOptsMask,
) *Broker {
	theBroker := Broker{
		brokerType:              brokerType,
		logger:                  logger,
		os:                      os,
		mutex:                   &sync.Mutex{},
		clock:                   clock,
		store:                   store,
		services:                services,
		configMask:              configMask,
		DisallowedBindOverrides: []string{SHARE_KEY, SOURCE_KEY},
	}

	return &theBroker
}

func (b *Broker) isNFSBroker() bool {
	return b.brokerType == BrokerTypeNFS
}

func (b *Broker) isSMBBroker() bool {
	return b.brokerType == BrokerTypeSMB
}

func (b *Broker) Services(_ context.Context) ([]brokerapi.Service, error) {
	logger := b.logger.Session("services")
	logger.Info("start")
	defer logger.Info("end")

	return b.services.List(), nil
}

func (b *Broker) Provision(context context.Context, instanceID string, details brokerapi.ProvisionDetails, asyncAllowed bool) (_ brokerapi.ProvisionedServiceSpec, e error) {
	logger := b.logger.Session("provision").WithData(lager.Data{"instanceID": instanceID, "details": details})
	logger.Info("start")
	defer logger.Info("end")

	var configuration map[string]interface{}

	var decoder = json.NewDecoder(bytes.NewBuffer(details.RawParameters))
	err := decoder.Decode(&configuration)
	if err != nil {
		return brokerapi.ProvisionedServiceSpec{}, brokerapi.ErrRawParamsInvalid
	}

	share := stringifyShare(configuration[SHARE_KEY])
	if share == "" {
		return brokerapi.ProvisionedServiceSpec{}, errors.New("config requires a \"share\" key")
	}


	if _, ok := configuration[SOURCE_KEY]; ok {
		return brokerapi.ProvisionedServiceSpec{}, errors.New("create configuration contains the following invalid option: ['" + SOURCE_KEY + "']")
	}

	if b.isNFSBroker() {
		re := regexp.MustCompile("^[^/]+:/")
		match := re.MatchString(share)

		if match {
			return brokerapi.ProvisionedServiceSpec{}, errors.New("syntax error for share: no colon allowed after server")
		}
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()
	defer func() {
		out := b.store.Save(logger)
		if e == nil {
			e = out
		}
	}()

	instanceDetails := brokerstore.ServiceInstance{
		ServiceID:          details.ServiceID,
		PlanID:             details.PlanID,
		OrganizationGUID:   details.OrganizationGUID,
		SpaceGUID:          details.SpaceGUID,
		ServiceFingerPrint: configuration,
	}

	if b.instanceConflicts(instanceDetails, instanceID) {
		return brokerapi.ProvisionedServiceSpec{}, brokerapi.ErrInstanceAlreadyExists
	}

	err = b.store.CreateInstanceDetails(instanceID, instanceDetails)
	if err != nil {
		return brokerapi.ProvisionedServiceSpec{}, fmt.Errorf("failed to store instance details: %s", err.Error())
	}

	logger.Info("service-instance-created", lager.Data{"instanceDetails": instanceDetails})

	return brokerapi.ProvisionedServiceSpec{IsAsync: false}, nil
}

func (b *Broker) Deprovision(context context.Context, instanceID string, details brokerapi.DeprovisionDetails, asyncAllowed bool) (_ brokerapi.DeprovisionServiceSpec, e error) {
	logger := b.logger.Session("deprovision")
	logger.Info("start")
	defer logger.Info("end")

	b.mutex.Lock()
	defer b.mutex.Unlock()
	defer func() {
		out := b.store.Save(logger)
		if e == nil {
			e = out
		}
	}()

	_, err := b.store.RetrieveInstanceDetails(instanceID)
	if err != nil {
		return brokerapi.DeprovisionServiceSpec{}, brokerapi.ErrInstanceDoesNotExist
	}

	err = b.store.DeleteInstanceDetails(instanceID)
	if err != nil {
		return brokerapi.DeprovisionServiceSpec{}, err
	}

	return brokerapi.DeprovisionServiceSpec{IsAsync: false, OperationData: "deprovision"}, nil
}

func (b *Broker) Bind(context context.Context, instanceID string, bindingID string, bindDetails brokerapi.BindDetails) (_ brokerapi.Binding, e error) {
	logger := b.logger.Session("bind")
	logger.Info("start", lager.Data{"bindingID": bindingID, "details": bindDetails})
	defer logger.Info("end")

	b.mutex.Lock()
	defer b.mutex.Unlock()
	defer func() {
		out := b.store.Save(logger)
		if e == nil {
			e = out
		}
	}()

	logger.Info("starting-broker-bind")
	instanceDetails, err := b.store.RetrieveInstanceDetails(instanceID)
	if err != nil {
		return brokerapi.Binding{}, brokerapi.ErrInstanceDoesNotExist
	}

	if bindDetails.AppGUID == "" {
		return brokerapi.Binding{}, brokerapi.ErrAppGuidNotProvided
	}

	opts, err := getFingerprint(instanceDetails.ServiceFingerPrint)
	if err != nil {
		return brokerapi.Binding{}, err
	}

	var bindOpts map[string]interface{}
	if len(bindDetails.RawParameters) > 0 {
		if err = json.Unmarshal(bindDetails.RawParameters, &bindOpts); err != nil {
			return brokerapi.Binding{}, err
		}
	}

	for k, v := range bindOpts {
		for _, disallowed := range b.DisallowedBindOverrides {
			if k == disallowed {
				err := errors.New(fmt.Sprintf("bind configuration contains the following invalid option: ['%s']", k))
				logger.Error("err-override-not-allowed-in-bind", err, lager.Data{"key": k})
				return brokerapi.Binding{}, brokerapi.NewFailureResponse(
					err, http.StatusUnprocessableEntity, "invalid-raw-params",
				)

			}
		}
		opts[k] = v
	}

	mode, err := evaluateMode(opts)
	if err != nil {
		logger.Error("error-evaluating-mode", err)
		return brokerapi.Binding{}, err
	}

	mountOpts, err := vmo.NewMountOpts(opts, b.configMask)
	if err != nil {
		logger.Error("error-generating-mount-options", err)
		return brokerapi.Binding{}, err
	}

	if b.bindingConflicts(bindingID, bindDetails) {
		return brokerapi.Binding{}, brokerapi.ErrBindingAlreadyExists
	}

	logger.Info("retrieved-instance-details", lager.Data{"instanceDetails": instanceDetails})

	err = b.store.CreateBindingDetails(bindingID, bindDetails)
	if err != nil {
		return brokerapi.Binding{}, err
	}

	driverName := "smbdriver"
	if b.isNFSBroker() {
		driverName = "nfsv3driver"

		// for backwards compatibility the nfs flavor has to issue source strings
		// with nfs:// prefix (otherwise the mapfs-mounter wont construct the correct
		// mount string for the kernel mount
		//
		// see (https://github.com/cloudfoundry/nfsv3driver/blob/ac1e1d26fec9a8551cacfabafa6e035f233c83e0/mapfs_mounter.go#L121)
		mountOpts[SOURCE_KEY] = fmt.Sprintf("nfs://%s", mountOpts[SOURCE_KEY])
	}

	logger.Debug("volume-service-binding", lager.Data{"driver": driverName, "mountOpts": mountOpts})

	s, err := b.hash(mountOpts)
	if err != nil {
		logger.Error("error-calculating-volume-id", err, lager.Data{"config": mountOpts, "bindingID": bindingID, "instanceID": instanceID})
		return brokerapi.Binding{}, err
	}
	volumeId := fmt.Sprintf("%s-%s", instanceID, s)

	mountConfig := map[string]interface{}{}

	for k, v := range mountOpts {
		mountConfig[k] = v
	}

	ret := brokerapi.Binding{
		Credentials: struct{}{}, // if nil, cloud controller chokes on response
		VolumeMounts: []brokerapi.VolumeMount{{
			ContainerDir: evaluateContainerPath(opts, instanceID),
			Mode:         mode,
			Driver:       driverName,
			DeviceType:   "shared",
			Device: brokerapi.SharedDevice{
				VolumeId:    volumeId,
				MountConfig: mountConfig,
			},
		}},
	}
	return ret, nil
}

func (b *Broker) hash(mountOpts map[string]interface{}) (string, error) {
	var (
		bytes []byte
		err   error
	)
	if bytes, err = json.Marshal(mountOpts); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", md5.Sum(bytes)), nil
}

func (b *Broker) Unbind(context context.Context, instanceID string, bindingID string, details brokerapi.UnbindDetails) (e error) {
	logger := b.logger.Session("unbind")
	logger.Info("start")
	defer logger.Info("end")

	b.mutex.Lock()
	defer b.mutex.Unlock()
	defer func() {
		out := b.store.Save(logger)
		if e == nil {
			e = out
		}
	}()

	if _, err := b.store.RetrieveInstanceDetails(instanceID); err != nil {
		return brokerapi.ErrInstanceDoesNotExist
	}

	if _, err := b.store.RetrieveBindingDetails(bindingID); err != nil {
		return brokerapi.ErrBindingDoesNotExist
	}

	if err := b.store.DeleteBindingDetails(bindingID); err != nil {
		return err
	}
	return nil
}

func (b *Broker) Update(context context.Context, instanceID string, details brokerapi.UpdateDetails, asyncAllowed bool) (brokerapi.UpdateServiceSpec, error) {
	return brokerapi.UpdateServiceSpec{},
		brokerapi.NewFailureResponse(
			errors.New("This service does not support instance updates. Please delete your service instance and create a new one with updated configuration."),
			422,
			"",
		)
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

func (b *Broker) instanceConflicts(details brokerstore.ServiceInstance, instanceID string) bool {
	return b.store.IsInstanceConflict(instanceID, brokerstore.ServiceInstance(details))
}

func (b *Broker) bindingConflicts(bindingID string, details brokerapi.BindDetails) bool {
	return b.store.IsBindingConflict(bindingID, details)
}

func evaluateContainerPath(parameters map[string]interface{}, volId string) string {
	if containerPath, ok := parameters["mount"]; ok && containerPath != "" {
		return containerPath.(string)
	}

	return path.Join(DEFAULT_CONTAINER_PATH, volId)
}

func evaluateMode(parameters map[string]interface{}) (string, error) {
	if ro, ok := parameters["readonly"]; ok {
		roc := vmou.InterfaceToString(ro)
		if roc == "true" {
			return "r", nil
		}

		return "", brokerapi.NewFailureResponse(fmt.Errorf("Invalid ro parameter value: %q", roc), http.StatusBadRequest, "invalid-ro-param")
	}

	return "rw", nil
}

func getFingerprint(rawObject interface{}) (map[string]interface{}, error) {
	fingerprint, ok := rawObject.(map[string]interface{})
	if ok {
		return fingerprint, nil
	} else {
		// legacy service instances only store the "share" key in the service fingerprint.
		share, ok := rawObject.(string)
		if ok {
			return map[string]interface{}{SHARE_KEY: share}, nil
		}
		return nil, errors.New("unable to deserialize service fingerprint")
	}
}

func stringifyShare(data interface{}) string {
	if val, ok := data.(string); ok {
		return val
	}

	return ""
}
