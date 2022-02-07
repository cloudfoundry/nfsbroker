module code.cloudfoundry.org/existingvolumebroker

require (
	code.cloudfoundry.org/clock v1.0.0
	code.cloudfoundry.org/goshims v0.10.0
	code.cloudfoundry.org/lager v2.0.0+incompatible
	code.cloudfoundry.org/service-broker-store v0.28.0
	code.cloudfoundry.org/volume-mount-options v1.1.0
	github.com/google/gofuzz v1.2.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.18.1
	github.com/pivotal-cf/brokerapi v6.4.2+incompatible
	github.com/tedsuo/ifrit v0.0.0-20191009134036-9a97d0632f00
)

go 1.13
