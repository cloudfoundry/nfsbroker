module code.cloudfoundry.org/existingvolumebroker

require (
	code.cloudfoundry.org/clock v0.0.0-20180518195852-02e53af36e6c
	code.cloudfoundry.org/goshims v0.0.0-20190529192408-bb24d2ef71ff
	code.cloudfoundry.org/lager v2.0.0+incompatible
	code.cloudfoundry.org/service-broker-store v0.0.0-20191022224831-b7b13dc8c343
	code.cloudfoundry.org/volume-mount-options v0.0.0-20191112224024-20b5adcd41a3
	github.com/google/gofuzz v1.1.0
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/pivotal-cf/brokerapi v2.0.5+incompatible
	github.com/tedsuo/ifrit v0.0.0-20191009134036-9a97d0632f00
)

go 1.13
