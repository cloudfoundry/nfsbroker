module code.cloudfoundry.org/existingvolumebroker

require (
	code.cloudfoundry.org/clock v1.0.0
	code.cloudfoundry.org/goshims v0.1.0
	code.cloudfoundry.org/lager v2.0.0+incompatible
	code.cloudfoundry.org/service-broker-store v0.3.0
	code.cloudfoundry.org/volume-mount-options v1.1.0
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/golang/protobuf v1.3.5 // indirect
	github.com/google/gofuzz v1.1.0
	github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega v1.9.0
	github.com/pivotal-cf/brokerapi v6.4.2+incompatible
	github.com/tedsuo/ifrit v0.0.0-20191009134036-9a97d0632f00
	golang.org/x/crypto v0.0.0-20200317142112-1b76d66859c6 // indirect
	golang.org/x/sys v0.0.0-20200317113312-5766fd39f98d // indirect
)

go 1.13
