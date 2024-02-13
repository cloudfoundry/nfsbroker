package stores

import "code.cloudfoundry.org/service-broker-store/brokerstore"

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6@latest -o ../fakes/retired_store_fake.go . RetiredStore
type RetiredStore interface {
	IsRetired() (bool, error)
	brokerstore.Store
}
