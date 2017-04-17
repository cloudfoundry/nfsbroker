package nfsbroker_test

import (
	"fmt"
	"strconv"
	"strings"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	. "code.cloudfoundry.org/nfsbroker/nfsbroker"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func map2string(entry map[string]string, joinKeyVal string, prefix string, joinElemnts string) string {
	return strings.Join(map2slice(entry, joinKeyVal, prefix), joinElemnts)
}

func mapstring2mapinterface(entry map[string]string) map[string]interface{} {
	result := make(map[string]interface{}, 0)

	for k, v := range entry {
		result[k] = v
	}

	return result
}

func map2slice(entry map[string]string, joinKeyVal string, prefix string) []string {
	result := make([]string, 0)

	for k, v := range entry {
		result = append(result, fmt.Sprintf("%s%s%s%s", prefix, k, joinKeyVal, v))
	}

	return result
}

func mapint2slice(entry map[string]interface{}, joinKeyVal string, prefix string) []string {
	result := make([]string, 0)

	for k, v := range entry {
		switch v.(type) {
		case int:
			result = append(result, fmt.Sprintf("%s%s%s%s", prefix, k, joinKeyVal, strconv.FormatInt(int64(v.(int)), 10)))

		case string:
			result = append(result, fmt.Sprintf("%s%s%s%s", prefix, k, joinKeyVal, v.(string)))

		case bool:
			result = append(result, fmt.Sprintf("%s%s%s%s", prefix, k, joinKeyVal, strconv.FormatBool(v.(bool))))
		}

	}

	return result
}

func inSliceString(list []string, val string) bool {
	for _, v := range list {
		if v == val {
			return true
		}
	}

	return false
}

func inMapInt(list map[string]interface{}, key string, val interface{}) bool {
	for k, v := range list {
		if k != key {
			continue
		}

		if v == val {
			return true
		} else {
			return false
		}
	}

	return false
}

var _ = Describe("BrokerConfigDetails", func() {
	var (
		logger lager.Logger

		clientShare     string
		arbitraryConfig  map[string]interface{}
		ignoreConfigKey []string

		allowed      []string
		mountOptions map[string]string

		configDetails *ConfigDetails
		config        *Config

		errorEntries error
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test-broker-config")
	})

	Context("Given empty params", func() {
		BeforeEach(func() {
			clientShare = "nfs://1.2.3.4"
			arbitraryConfig = make(map[string]interface{}, 0)
			ignoreConfigKey = make([]string, 0)

			allowed = make([]string, 0)
			mountOptions = make(map[string]string, 0)

			configDetails = NewNfsBrokerConfigDetails()
			configDetails.ReadConf(strings.Join(allowed, ","), map2string(mountOptions, ":", "", ","))

			config = NewNfsBrokerConfig(configDetails)
			logger.Debug("debug-config-initiated", lager.Data{"mount": configDetails})

			errorEntries = config.SetEntries(logger, clientShare, arbitraryConfig, ignoreConfigKey)
			logger.Debug("debug-config-updated", lager.Data{"mount": configDetails})
		})

		It("should returns empty allowed list", func() {
			Expect(len(configDetails.Allowed)).To(Equal(0))
		})

		It("should returns empty forced list", func() {
			Expect(len(configDetails.Forced)).To(Equal(0))
		})

		It("should returns empty options list", func() {
			Expect(len(configDetails.Options)).To(Equal(0))
		})

		It("should flow sloppy_mount as disabled", func() {
			Expect(configDetails.IsSloppyMount()).To(BeFalse())
		})

		It("should returns no error on given client arbitrary config", func() {
			Expect(errorEntries).To(BeNil())
		})

		It("should returns no MountOptions struct", func() {
			Expect(len(config.MountConfig())).To(Equal(0))
		})

		It("returns no added parameters to the client share", func() {
			Expect(config.Share(clientShare)).To(Equal(clientShare))
		})
	})

	Context("Given allowed and default params", func() {
		BeforeEach(func() {
			clientShare = "nfs://1.2.3.4"
			arbitraryConfig = make(map[string]interface{}, 0)
			ignoreConfigKey = make([]string, 0)

			allowed = []string{"sloppy_mount", "nfs_uid", "nfs_gid", "allow_other", "uid", "gid", "auto-traverse-mounts", "dircache", "foo"}
			mountOptions = map[string]string{
				"nfs_uid": "1003",
				"nfs_gid": "1001",
				"uid":     "1004",
				"gid":     "1002",
			}

			configDetails = NewNfsBrokerConfigDetails()
			configDetails.ReadConf(strings.Join(allowed, ","), map2string(mountOptions, ":", "", ","))

			config = NewNfsBrokerConfig(configDetails)
			logger.Debug("debug-config-initiated", lager.Data{"config": config, "mount": configDetails})
		})

		It("should flow the allowed list", func() {
			Expect(configDetails.Allowed).To(Equal(allowed))
		})

		It("should return empty forced list", func() {
			Expect(len(configDetails.Forced)).To(Equal(0))
		})

		It("should flow the default params as options list", func() {
			Expect(configDetails.Options).To(Equal(mountOptions))
		})

		It("should flow sloppy_mount as disabled", func() {
			Expect(configDetails.IsSloppyMount()).To(BeFalse())
		})

		Context("Given empty arbitrary params and share without any params", func() {
			BeforeEach(func() {
				errorEntries = config.SetEntries(logger, clientShare, arbitraryConfig, ignoreConfigKey)
				logger.Debug("debug-config-updated", lager.Data{"config": config, "mount": configDetails})
			})

			It("should return nil result on setting end users'entries", func() {
				Expect(errorEntries).To(BeNil())
			})

			It("should pass the default options into the MountOptions struct", func() {
				actualRes := config.MountConfig()
				expectRes := mapstring2mapinterface(mountOptions)

				for k, exp := range expectRes {
					logger.Debug("checking-expect-res-contains-part", lager.Data{"expectRes": expectRes, "key": k, "val": exp})
					Expect(inMapInt(actualRes, k, exp)).To(BeTrue())
				}

				for k, exp := range actualRes {
					logger.Debug("checking-expect-res-contains-part", lager.Data{"expectRes": expectRes, "key": k, "val": exp})
					Expect(inMapInt(expectRes, k, exp)).To(BeTrue())
				}
			})

			It("should pass no options into the client share url", func() {
				share := config.Share(clientShare)
				Expect(share).NotTo(ContainSubstring(clientShare + "?"))
			})
		})

		Context("Given bad arbitrary params and bad share params", func() {

			BeforeEach(func() {
				clientShare = "nfs://1.2.3.4?err=true&test=err"
				arbitraryConfig = map[string]interface{}{
					"missing": true,
					"wrong":   1234,
					"search":  "notfound",
				}
				ignoreConfigKey = make([]string, 0)

				errorEntries = config.SetEntries(logger, clientShare, arbitraryConfig, ignoreConfigKey)
				logger.Debug("debug-config-updated", lager.Data{"config": config, "mount": configDetails})
			})

			It("should return an error", func() {
				Expect(errorEntries).To(HaveOccurred())
				logger.Debug("debug-config-updated with entry", lager.Data{"config": config, "mount": configDetails})
			})

			It("should flow the mount default options into the MountOptions struct", func() {
				actualRes := config.MountConfig()
				expectRes := mapstring2mapinterface(mountOptions)

				for k, exp := range expectRes {
					logger.Debug("checking-expect-res-contains-part", lager.Data{"expectRes": expectRes, "key": k, "val": exp})
					Expect(inMapInt(actualRes, k, exp)).To(BeTrue())
				}

				for k, exp := range actualRes {
					logger.Debug("checking-expect-res-contains-part", lager.Data{"expectRes": expectRes, "key": k, "val": exp})
					Expect(inMapInt(expectRes, k, exp)).To(BeTrue())
				}
			})

			It("should remove all options from the client share url", func() {
				share := config.Share(clientShare)
				Expect(share).To(Equal("nfs://1.2.3.4"))
			})
		})

		Context("Given bad params with sloppy_mount mode", func() {
			BeforeEach(func() {
				clientShare = "nfs://1.2.3.4"
				arbitraryConfig = map[string]interface{}{
					"sloppy_mount":         true,
					"allow_other":          true,
					"auto-traverse-mounts": true,
					"dircache":             false,
					"missing":              true,
					"wrong":                1234,
					"search":               "notfound",
				}
				ignoreConfigKey = make([]string, 0)

				errorEntries = config.SetEntries(logger, clientShare, arbitraryConfig, ignoreConfigKey)
				logger.Debug("debug-config-updated", lager.Data{"config": config, "mount": configDetails})
			})

			It("should not error", func() {
				Expect(errorEntries).To(BeNil())
				logger.Debug("debug-config-updated with entry", lager.Data{"config": config, "mount": configDetails})
			})

			It("should flow the mount default options into the MountOptions struct", func() {
				actualRes := config.MountConfig()

				expectRes := mapstring2mapinterface(mountOptions)
				expectRes["allow_other"] = "true"
				expectRes["auto-traverse-mounts"] = "1"
				expectRes["dircache"] = "0"

				for k, exp := range expectRes {
					logger.Debug("checking-expect-res-contains-part", lager.Data{"actualRes": actualRes, "key": k, "val": exp})
					Expect(inMapInt(actualRes, k, exp)).To(BeTrue())
				}

				for k, exp := range actualRes {
					logger.Debug("checking-expect-res-contains-part", lager.Data{"expectRes": expectRes, "key": k, "val": exp})
					Expect(inMapInt(expectRes, k, exp)).To(BeTrue())
				}
			})
		})

		Context("Given good arbitrary params", func() {

			BeforeEach(func() {
				clientShare = "nfs://1.2.3.4"
				arbitraryConfig = map[string]interface{}{
					"nfs_uid": "1234",
					"nfs_gid": "5678",
					"uid":     "2999",
					"gid":     "1999",
				}
				ignoreConfigKey = make([]string, 0)

				errorEntries = config.SetEntries(logger, clientShare, arbitraryConfig, ignoreConfigKey)
				logger.Debug("debug-config-updated", lager.Data{"config": config, "mount": configDetails})
			})

			It("should not error", func() {
				Expect(errorEntries).To(BeNil())
			})

			It("should flow the arbitrary config into the MountOptions struct", func() {
				actualRes := config.MountConfig()
				expectRes := arbitraryConfig

				for k, exp := range expectRes {
					logger.Debug("checking-expect-res-contains-part", lager.Data{"expectRes": expectRes, "key": k, "val": exp})
					Expect(inMapInt(actualRes, k, exp)).To(BeTrue())
				}

				for k, exp := range actualRes {
					logger.Debug("checking-expect-res-contains-part", lager.Data{"expectRes": expectRes, "key": k, "val": exp})
					Expect(inMapInt(expectRes, k, exp)).To(BeTrue())
				}
			})

			It("should not modify the mount share url", func() {
				share := config.Share(clientShare)
				Expect(share).To(Equal(clientShare))
			})

			Context("when the whole config is copied", func() {
				var config2 *Config
				BeforeEach(func() {
					config2 = config.Copy()
				})
				It("should flow the arbitrary config into the MountOptions struct", func() {
					actualRes := config2.MountConfig()
					expectRes := arbitraryConfig

					for k, exp := range expectRes {
						logger.Debug("checking-expect-res-contains-part", lager.Data{"expectRes": expectRes, "key": k, "val": exp})
						Expect(inMapInt(actualRes, k, exp)).To(BeTrue())
					}

					for k, exp := range actualRes {
						logger.Debug("checking-expect-res-contains-part", lager.Data{"expectRes": expectRes, "key": k, "val": exp})
						Expect(inMapInt(expectRes, k, exp)).To(BeTrue())
					}
				})

				It("should not modify the mount share url", func() {
					share := config2.Share(clientShare)
					Expect(share).To(Equal(clientShare))
				})
			})
		})

		Context("Given good arbitrary params with integer values", func() {

			BeforeEach(func() {
				clientShare = "nfs://1.2.3.4"
				var fooval int64
				fooval = 56
				arbitraryConfig = map[string]interface{}{
					"uid":     2999,
					"gid":     1999,
					"foo":		 fooval,
				}
				ignoreConfigKey = make([]string, 0)

				errorEntries = config.SetEntries(logger, clientShare, arbitraryConfig, ignoreConfigKey)
				logger.Debug("debug-config-updated", lager.Data{"config": config, "mount": configDetails})
			})

			It("should not error", func() {
				Expect(errorEntries).To(BeNil())
			})

			It("should flow the arbitrary config into the MountOptions struct", func() {
				actualRes := config.MountConfig()

				v, ok := actualRes["foo"]

				Expect(ok).To(BeTrue())
				Expect(v).To(Equal("56"))
			})
		})
	})
})
