module code.cloudfoundry.org/nfsbroker

go 1.12

require (
	code.cloudfoundry.org/clock v0.0.0-20180518195852-02e53af36e6c
	code.cloudfoundry.org/debugserver v0.0.0-20180612203758-a3ba348dfede
	code.cloudfoundry.org/existingvolumebroker v0.0.0-20191105200250-b1c4bfa69d43
	code.cloudfoundry.org/goshims v0.0.0-20190529192408-bb24d2ef71ff
	code.cloudfoundry.org/lager v2.0.0+incompatible
	code.cloudfoundry.org/service-broker-store v0.0.0-20191022224831-b7b13dc8c343
	code.cloudfoundry.org/volume-mount-options v0.0.0-20191112224024-20b5adcd41a3
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d // indirect
	github.com/onsi/ginkgo v1.10.3
	github.com/onsi/gomega v1.7.1
	github.com/pivotal-cf/brokerapi v2.0.5+incompatible
	github.com/tedsuo/ifrit v0.0.0-20191009134036-9a97d0632f00
)
