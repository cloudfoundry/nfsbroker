module code.cloudfoundry.org/nfsbroker

go 1.13

require (
	code.cloudfoundry.org/clock v1.0.0
	code.cloudfoundry.org/debugserver v0.0.0-20200131002057-141d5fa0e064
	code.cloudfoundry.org/existingvolumebroker v0.28.0
	code.cloudfoundry.org/goshims v0.4.0
	code.cloudfoundry.org/lager v2.0.0+incompatible
	code.cloudfoundry.org/service-broker-store v0.10.0
	code.cloudfoundry.org/volume-mount-options v1.1.0
	github.com/google/gofuzz v1.2.0
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d // indirect
	github.com/onsi/ginkgo v1.14.2
	github.com/onsi/gomega v1.10.4
	github.com/pivotal-cf/brokerapi v6.4.2+incompatible
	github.com/tedsuo/ifrit v0.0.0-20191009134036-9a97d0632f00
)
