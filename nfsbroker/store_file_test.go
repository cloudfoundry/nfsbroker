package nfsbroker_test

import (
	"errors"

	"code.cloudfoundry.org/goshims/ioutilshim/ioutil_fake"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/nfsbroker/nfsbroker"
	"github.com/pivotal-cf/brokerapi"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("FileStore", func() {
	var (
		store      nfsbroker.Store
		fakeIoutil *ioutil_fake.FakeIoutil
		logger     lager.Logger
		state      nfsbroker.DynamicState
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test-broker")
		fakeIoutil = &ioutil_fake.FakeIoutil{}
		store = nfsbroker.NewFileStore("/tmp/whatever", fakeIoutil)
		state = nfsbroker.DynamicState{
			InstanceMap: map[string]nfsbroker.ServiceInstance{
				"service-name": {
					Share: "server:/some-share",
				},
			},
			BindingMap: map[string]brokerapi.BindDetails{},
		}
	})

	Describe("Restore", func() {
		var (
			err error
		)

		Context("when it succeeds", func() {
			BeforeEach(func() {
				fakeIoutil.ReadFileReturns([]byte(`{"InstanceMap":{},"BindingMap":{}}`), nil)
				err = store.Restore(logger, &state)
			})

			It("reads the file", func() {
				Expect(fakeIoutil.ReadFileCallCount()).To(Equal(1))
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when the file system is failing", func() {
			BeforeEach(func() {
				fakeIoutil.ReadFileReturns(nil, errors.New("badness"))
				err = store.Restore(logger, &state)
			})

			It("returns an error", func() {
				Expect(err).To(MatchError("badness"))
			})
		})
		Context("when there is junk in the file", func() {
			BeforeEach(func() {
				filecontents := "{serviceName: [some invalid state]}"
				fakeIoutil.ReadFileReturns([]byte(filecontents), nil)
				err = store.Restore(logger, &state)
			})
			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Save", func() {
		var (
			err error
		)

		Context("when it succeeds", func() {
			BeforeEach(func() {
				fakeIoutil.WriteFileReturns(nil)
				err = store.Save(logger, &state, "", "")
			})

			It("writes the file", func() {
				Expect(fakeIoutil.WriteFileCallCount()).To(Equal(1))
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when the file system is failing", func() {
			BeforeEach(func() {
				fakeIoutil.WriteFileReturns(errors.New("badness"))
				err = store.Save(logger, &state, "", "")
			})

			It("returns an error", func() {
				Expect(err).To(MatchError("badness"))
			})
		})
	})

	Describe("Cleanup", func() {
		var (
			err error
		)

		Context("when it succeeds", func() {
			BeforeEach(func() {
				err = store.Cleanup()
			})

			It("doesn't error", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

})
