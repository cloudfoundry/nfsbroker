package main

import (
	"code.cloudfoundry.org/existingvolumebroker"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/debugserver"
	"code.cloudfoundry.org/goshims/osshim"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagerflags"
	"code.cloudfoundry.org/nfsbroker/utils"
	"code.cloudfoundry.org/service-broker-store/brokerstore"
	vmo "code.cloudfoundry.org/volume-mount-options"
	vmou "code.cloudfoundry.org/volume-mount-options/utils"
	"github.com/pivotal-cf/brokerapi"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
)

var dataDir = flag.String(
	"dataDir",
	"",
	"[REQUIRED] - Broker's state will be stored here to persist across reboots",
)

var atAddress = flag.String(
	"listenAddr",
	"0.0.0.0:8999",
	"host:port to serve service broker API",
)

var servicesConfig = flag.String(
	"servicesConfig",
	"",
	"[REQUIRED] - Path to services config to register with cloud controller",
)

var dbDriver = flag.String(
	"dbDriver",
	"",
	"(optional) database driver name when using SQL to store broker state",
)

var dbHostname = flag.String(
	"dbHostname",
	"",
	"(optional) database hostname when using SQL to store broker state",
)
var dbPort = flag.String(
	"dbPort",
	"",
	"(optional) database port when using SQL to store broker state",
)

var dbName = flag.String(
	"dbName",
	"",
	"(optional) database name when using SQL to store broker state",
)

var dbCACertPath = flag.String(
	"dbCACertPath",
	"",
	"(optional) Path to CA Cert for database SSL connection",
)

var dbSkipHostnameValidation = flag.Bool(
	"dbSkipHostnameValidation",
	false,
	"(optional) skip DB server hostname validation when connecting over TLS",
)

var cfServiceName = flag.String(
	"cfServiceName",
	"",
	"(optional) For CF pushed apps, the service name in VCAP_SERVICES where we should find database credentials.  dbDriver must be defined if this option is set, but all other db parameters will be extracted from the service binding.",
)

var allowedOptions = flag.String(
	"allowedOptions",
	"auto_cache,uid,gid",
	"A comma separated list of parameters allowed to be set in config.",
)

var defaultOptions = flag.String(
	"defaultOptions",
	"auto_cache:true",
	"A comma separated list of defaults specified as param:value. If a parameter has a default value and is not in the allowed list, this default value becomes a fixed value that cannot be overridden",
)

var credhubURL = flag.String(
	"credhubURL",
	"",
	"(optional) CredHub server URL when using CredHub to store broker state",
)

var credhubCACertPath = flag.String(
	"credhubCACertPath",
	"",
	"(optional) Path to CA Cert for CredHub",
)

var uaaClientID = flag.String(
	"uaaClientID",
	"",
	"(optional) UAA client ID when using CredHub to store broker state",
)

var uaaClientSecret = flag.String(
	"uaaClientSecret",
	"",
	"(optional) UAA client secret when using CredHub to store broker state",
)

var uaaCACertPath = flag.String(
	"uaaCACertPath",
	"",
	"(optional) Path to CA Cert for UAA used for CredHub authorization",
)

var storeID = flag.String(
	"storeID",
	"nfsbroker",
	"(optional) Store ID used to namespace instance details and bindings (credhub only)",
)

var (
	username   string
	password   string
	dbUsername string
	dbPassword string
)

//go:generate counterfeiter -o fakes/retired_store_fake.go . RetiredStore
type RetiredStore interface {
	IsRetired() (bool, error)
	brokerstore.Store
}

func main() {
	parseCommandLine()
	parseEnvironment()

	checkParams()

	logger, logSink := newLogger()
	logger.Info("starting")
	defer logger.Info("ends")

	server := createServer(logger)

	if dbgAddr := debugserver.DebugAddress(flag.CommandLine); dbgAddr != "" {
		server = utils.ProcessRunnerFor(grouper.Members{
			{Name: "debug-server", Runner: debugserver.Runner(dbgAddr, logSink)},
			{Name: "broker-api", Runner: server},
		})
	}

	process := ifrit.Invoke(server)
	logger.Info("started")
	utils.UntilTerminated(logger, process)
}

func parseCommandLine() {
	lagerflags.AddFlags(flag.CommandLine)
	debugserver.AddFlags(flag.CommandLine)
	flag.Parse()
}

func parseEnvironment() {
	username, _ = os.LookupEnv("USERNAME")
	password, _ = os.LookupEnv("PASSWORD")
	dbUsername, _ = os.LookupEnv("DB_USERNAME")
	dbPassword, _ = os.LookupEnv("DB_PASSWORD")
}

func checkParams() {
	if *dataDir == "" && *dbDriver == "" && *credhubURL == "" {
		fmt.Fprint(os.Stderr, "\nERROR: Either dataDir, dbDriver or credhubURL parameters must be provided.\n\n")
		flag.Usage()
		os.Exit(1)
	}

	if *servicesConfig == "" {
		fmt.Fprint(os.Stderr, "\nERROR: servicesConfig parameter must be provided.\n\n")
		flag.Usage()
		os.Exit(1)
	}
}

func newLogger() (lager.Logger, *lager.ReconfigurableSink) {
	lagerConfig := lagerflags.ConfigFromFlags()
	lagerConfig.RedactSecrets = true

	return lagerflags.NewFromConfig("nfsbroker", lagerConfig)
}

func parseVcapServices(logger lager.Logger, os osshim.Os) {
	if *dbDriver == "" {
		logger.Fatal("missing-db-driver-parameter", errors.New("dbDriver parameter is required for cf deployed broker"))
	}

	// populate db parameters from VCAP_SERVICES and pitch a fit if there isn't one.
	services, hasValue := os.LookupEnv("VCAP_SERVICES")
	if !hasValue {
		logger.Fatal("missing-vcap-services-environment", errors.New("missing VCAP_SERVICES environment"))
	}

	stuff := map[string][]interface{}{}
	err := json.Unmarshal([]byte(services), &stuff)
	if err != nil {
		logger.Fatal("json-unmarshal-error", err)
	}

	stuff2, ok := stuff[*cfServiceName]
	if !ok {
		logger.Fatal("missing-service-binding", errors.New("VCAP_SERVICES missing specified db service"), lager.Data{"stuff": stuff})
	}

	stuff3 := stuff2[0].(map[string]interface{})

	credentials := stuff3["credentials"].(map[string]interface{})
	logger.Debug("credentials-parsed", lager.Data{"credentials": credentials})

	dbUsername = getByAlias(credentials, "user", "username").(string)
	dbPassword = getByAlias(credentials, "pass", "password").(string)
	*dbHostname = getByAlias(credentials, "host", "hostname").(string)
	if *dbPort, ok = getByAlias(credentials, "port").(string); !ok {
		*dbPort = fmt.Sprintf("%.0f", getByAlias(credentials, "port").(float64))
	}
	*dbName = getByAlias(credentials, "name", "db_name").(string)
}

func getByAlias(data map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		value, ok := data[key]
		if ok {
			return value
		}
	}
	return nil
}

func createServer(logger lager.Logger) ifrit.Runner {
	fileName := filepath.Join(*dataDir, fmt.Sprintf("nfs-services.json"))

	// if we are CF pushed
	if *cfServiceName != "" {
		parseVcapServices(logger, &osshim.OsShim{})
	}

	var dbCACert string
	if *dbCACertPath != "" {
		b, err := ioutil.ReadFile(*dbCACertPath)
		if err != nil {
			logger.Fatal("cannot-read-db-ca-cert", err, lager.Data{"path": *dbCACertPath})
		}
		dbCACert = string(b)
	}

	var credhubCACert string
	if *credhubCACertPath != "" {
		b, err := ioutil.ReadFile(*credhubCACertPath)
		if err != nil {
			logger.Fatal("cannot-read-credhub-ca-cert", err, lager.Data{"path": *credhubCACertPath})
		}
		credhubCACert = string(b)
	}

	var uaaCACert string
	if *uaaCACertPath != "" {
		b, err := ioutil.ReadFile(*uaaCACertPath)
		if err != nil {
			logger.Fatal("cannot-read-credhub-ca-cert", err, lager.Data{"path": *uaaCACertPath})
		}
		uaaCACert = string(b)
	}

	store := brokerstore.NewStore(
		logger,
		*dbDriver,
		dbUsername,
		dbPassword,
		*dbHostname,
		*dbPort,
		*dbName,
		dbCACert,
		*dbSkipHostnameValidation,
		*credhubURL,
		credhubCACert,
		*uaaClientID,
		*uaaClientSecret,
		uaaCACert,
		fileName,
		*storeID,
	)

	retired, err := IsRetired(store)
	if err != nil {
		logger.Fatal("check-is-retired-failed", err)
	}

	if retired {
		logger.Fatal("retired-store", errors.New("Store is retired"))
	}

	configMask, err := vmo.NewMountOptsMask(
		strings.Split(*allowedOptions, ","),
		vmou.ParseOptionStringToMap(*defaultOptions, ":"),
		map[string]string{
			"readonly": "ro",
			"share":    "source",
		},
		[]string{},
		[]string{"source"},
	)
	if err != nil {
		logger.Fatal("creating-config-mask-error", err)
	}

	logger.Debug("nfsbroker-startup-config", lager.Data{"config-mask": configMask})

	services, err := NewServicesFromConfig(*servicesConfig)
	if err != nil {
		logger.Fatal("loading-services-config-error", err)
	}

	serviceBroker := existingvolumebroker.New(
		existingvolumebroker.BrokerTypeNFS,
		logger,
		services,
		&osshim.OsShim{},
		clock.NewClock(),
		store,
		configMask,
	)

	credentials := brokerapi.BrokerCredentials{Username: username, Password: password}
	handler := brokerapi.New(serviceBroker, logger.Session("broker-api"), credentials)

	return http_server.New(*atAddress, handler)
}

func IsRetired(store brokerstore.Store) (bool, error) {
	if retiredStore, ok := store.(RetiredStore); ok {
		return retiredStore.IsRetired()
	}
	return false, nil
}
