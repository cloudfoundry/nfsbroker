package main

import (
	"flag"
	"fmt"
	"os"

	"code.cloudfoundry.org/cflager"
	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/debugserver"
	"code.cloudfoundry.org/goshims/ioutilshim"
	"code.cloudfoundry.org/goshims/osshim"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/nfsbroker/nfsbroker"
	"code.cloudfoundry.org/nfsbroker/utils"

	"path/filepath"

	"code.cloudfoundry.org/goshims/sqlshim"
	"github.com/lib/pq"
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

var serviceName = flag.String(
	"serviceName",
	"knfsvolume",
	"name of the service to register with cloud controller",
)
var serviceId = flag.String(
	"serviceId",
	"service-guid",
	"ID of the service to register with cloud controller",
)
var username = flag.String(
	"username",
	"admin",
	"basic auth username to verify on incoming requests",
)
var password = flag.String(
	"password",
	"admin",
	"basic auth password to verify on incoming requests",
)
var dbDriver = flag.String(
	"dbDriver",
	"",
	"(optional) database driver name when using SQL to store broker state",
)

var dbUsername = flag.String(
	"dbUsername",
	"",
	"(optional) database username when using SQL to store broker state",
)
var dbPassword = flag.String(
	"dbPassword",
	"",
	"(optional) database password when using SQL to store broker state",
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

func main() {
	parseCommandLine()

	checkParams()

	logger, logSink := cflager.New("nfsbroker")
	logger.Info("starting")
	defer logger.Info("ends")

	server := createServer(logger)

	if dbgAddr := debugserver.DebugAddress(flag.CommandLine); dbgAddr != "" {
		server = utils.ProcessRunnerFor(grouper.Members{
			{"debug-server", debugserver.Runner(dbgAddr, logSink)},
			{"broker-api", server},
		})
	}

	process := ifrit.Invoke(server)
	logger.Info("started")
	utils.UntilTerminated(logger, process)
}

func parseCommandLine() {
	cflager.AddFlags(flag.CommandLine)
	debugserver.AddFlags(flag.CommandLine)
	flag.Parse()
}

func checkParams() {
	if *dataDir == "" {
		fmt.Fprint(os.Stderr, "\nERROR: Required parameter dataDir not defined.\n\n")
		flag.Usage()
		os.Exit(1)
	}

}

func createServer(logger lager.Logger) ifrit.Runner {
	fileName := filepath.Join(*dataDir, fmt.Sprintf("%s-services.json", *serviceName))
	var store nfsbroker.Store
	var err error
	if *dbDriver != "" {
		store, err = nfsbroker.NewSqlStore(logger, &sqlshim.SqlShim{}, *dbDriver, *dbUsername, *dbPassword, *dbHostname, *dbPort, *dbName)
		if err != nil {
			logger.Fatal("failed-creating-sql-store", err)
		}
	} else {
		store = nfsbroker.NewFileStore(fileName, &ioutilshim.IoutilShim{})
	}
	serviceBroker := nfsbroker.New(logger,
		*serviceName, *serviceId,
		*dataDir, &osshim.OsShim{}, clock.NewClock(), store)

	credentials := brokerapi.BrokerCredentials{Username: *username, Password: *password}
	handler := brokerapi.New(serviceBroker, logger.Session("broker-api"), credentials)

	return http_server.New(*atAddress, handler)
}

func ConvertPostgresError(err *pq.Error) string {
	return ""
}
