package nfsbrokerfakes

import (
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/goshims/sqlshim"
)

type FakeSQLMockConnection struct {
sqlshim.SqlDB
}

func (fake FakeSQLMockConnection) Connect(logger lager.Logger) error {
	return nil
}