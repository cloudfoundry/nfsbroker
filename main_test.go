package main

import (
	"errors"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"encoding/json"
	"io/ioutil"

	"fmt"

	"os"
	"time"

	"code.cloudfoundry.org/nfsbroker/fakes"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"
	"github.com/pivotal-cf/brokerapi"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var _ = Describe("nfsbroker Main", func() {
	Context("Parse VCAP_SERVICES tests", func() {

		BeforeEach(func() {
			*cfServiceName = "postgresql"
		})
	})

	Context("Missing required args", func() {
		var process ifrit.Process

		It("shows usage when dataDir or dbDriver are not provided", func() {
			var args []string
			volmanRunner := failRunner{
				Name:       "nfsbroker",
				Command:    exec.Command(binaryPath, args...),
				StartCheck: "Either dataDir or credhubURL parameters must be provided.",
			}
			process = ifrit.Invoke(volmanRunner)
		})

		It("shows usage when servicesConfig is not provided", func() {
			args := []string{"-credhubURL", "some-credhub"}
			volmanRunner := failRunner{
				Name:       "nfsbroker",
				Command:    exec.Command(binaryPath, args...),
				StartCheck: "servicesConfig parameter must be provided.",
			}
			process = ifrit.Invoke(volmanRunner)
		})

		AfterEach(func() {
			ginkgomon.Kill(process) // this is only if incorrect implementation leaves process running
		})
	})

	Context("credhub /info returns error", func() {
		var volmanRunner *ginkgomon.Runner
		var credhubServer *ghttp.Server

		table.DescribeTable("should log a helpful diagnostic error message ", func(statusCode int) {
			listenAddr := "0.0.0.0:" + strconv.Itoa(8999+GinkgoParallelNode())

			credhubServer = ghttp.NewServer()
			credhubServer.AppendHandlers(ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/info"),
				ghttp.RespondWith(statusCode, "", http.Header{"X-Squid-Err": []string{"some-error"}}),
			))
			defer credhubServer.Close()

			var args []string
			args = append(args, "-listenAddr", listenAddr)
			args = append(args, "-credhubURL", credhubServer.URL())
			args = append(args, "-servicesConfig", "./default_services.json")

			volmanRunner = ginkgomon.New(ginkgomon.Config{
				Name:       "nfsbroker",
				Command:    exec.Command(binaryPath, args...),
				StartCheck: "starting",
			})

			invoke := ifrit.Invoke(volmanRunner)
			defer ginkgomon.Kill(invoke)

			time.Sleep(2 * time.Second)
			Eventually(volmanRunner.ExitCode).Should(Equal(2))
			Eventually(volmanRunner.Buffer()).Should(gbytes.Say(fmt.Sprintf(".*Attempted to connect to credhub. Expected 200. Got %d.*X-Squid-Err:\\[some-error\\].*", statusCode)))

		},
			table.Entry("300", http.StatusMultipleChoices),
			table.Entry("400", http.StatusBadRequest),
			table.Entry("403", http.StatusForbidden),
			table.Entry("500", http.StatusInternalServerError))

		It("should timeout after 30 seconds", func() {
			listenAddr := "0.0.0.0:" + strconv.Itoa(8999+GinkgoParallelNode())

			var closeChan = make(chan interface{}, 1)

			credhubServer = ghttp.NewServer()
			credhubServer.AppendHandlers(ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/info"),
				func(w http.ResponseWriter, r *http.Request) {
					<-closeChan
				},
			))

			var args []string
			args = append(args, "-listenAddr", listenAddr)
			args = append(args, "-credhubURL", credhubServer.URL())
			args = append(args, "-servicesConfig", "./default_services.json")

			volmanRunner = ginkgomon.New(ginkgomon.Config{
				Name:       "nfsbroker",
				Command:    exec.Command(binaryPath, args...),
				StartCheck: "starting",
			})

			invoke := ifrit.Invoke(volmanRunner)
			defer func() {
				close(closeChan)
				credhubServer.Close()
				ginkgomon.Kill(invoke)
			}()

			Eventually(volmanRunner.ExitCode, "35s", "1s").Should(Equal(2))
			Eventually(volmanRunner.Buffer, "35s", "1s").Should(gbytes.Say(".*Unable to connect to credhub."))
		})
	})

	Context("Has required args", func() {
		var (
			args               []string
			listenAddr         string
			username, password string
			volmanRunner       *ginkgomon.Runner

			process ifrit.Process

			credhubServer *ghttp.Server
		)

		BeforeEach(func() {
			listenAddr = "0.0.0.0:" + strconv.Itoa(7999+GinkgoParallelNode())
			username = "admin"
			password = "password"

			os.Setenv("USERNAME", username)
			os.Setenv("PASSWORD", password)

			credhubServer = ghttp.NewServer()

			infoResponse := credhubInfoResponse{
				AuthServer: credhubInfoResponseAuthServer{
					URL: "some-auth-server-url",
				},
			}

			credhubServer.AppendHandlers(ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/info"),
				ghttp.RespondWithJSONEncoded(http.StatusOK, infoResponse),
			), ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/info"),
				ghttp.RespondWithJSONEncoded(http.StatusOK, infoResponse),
			))

			args = append(args, "-credhubURL", credhubServer.URL())
			args = append(args, "-listenAddr", listenAddr)
			args = append(args, "-servicesConfig", "./test_default_services.json")
		})

		JustBeforeEach(func() {
			volmanRunner = ginkgomon.New(ginkgomon.Config{
				Name:              "nfsbroker",
				Command:           exec.Command(binaryPath, args...),
				StartCheck:        "started",
				StartCheckTimeout: 20 * time.Second,
			})
			process = ginkgomon.Invoke(volmanRunner)
		})

		AfterEach(func() {
			ginkgomon.Kill(process)
		})

		httpDoWithAuth := func(method, endpoint string, body io.Reader) (*http.Response, error) {
			req, err := http.NewRequest(method, "http://"+listenAddr+endpoint, body)
			req.Header.Add("X-Broker-Api-Version", "2.14")
			Expect(err).NotTo(HaveOccurred())

			req.SetBasicAuth(username, password)
			return http.DefaultClient.Do(req)
		}

		It("should check for a proxy", func() {
			Eventually(volmanRunner.Buffer()).Should(gbytes.Say("no-proxy-found"))
		})

		It("should listen on the given address", func() {
			resp, err := httpDoWithAuth("GET", "/v2/catalog", nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(200))
		})

		It("should pass services config through to catalog", func() {
			resp, err := httpDoWithAuth("GET", "/v2/catalog", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			bytes, err := ioutil.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			var catalog brokerapi.CatalogResponse
			err = json.Unmarshal(bytes, &catalog)
			Expect(err).NotTo(HaveOccurred())

			Expect(catalog.Services).To(HaveLen(2))

			Expect(catalog.Services[0].Name).To(Equal("nfs-legacy"))
			Expect(catalog.Services[0].ID).To(Equal("nfsbroker"))
			Expect(catalog.Services[0].Plans[0].ID).To(Equal("Existing"))
			Expect(catalog.Services[0].Plans[0].Name).To(Equal("Existing"))
			Expect(catalog.Services[0].Plans[0].Description).To(Equal("A preexisting filesystem"))

			Expect(catalog.Services[1].Name).To(Equal("nfs"))
			Expect(catalog.Services[1].ID).To(Equal("997f8f26-e10c-11e7-80c1-9a214cf093ae"))
			Expect(catalog.Services[1].Plans[0].ID).To(Equal("09a09260-1df5-4445-9ed7-1ba56dadbbc8"))
			Expect(catalog.Services[1].Plans[0].Name).To(Equal("Existing"))
			Expect(catalog.Services[1].Plans[0].Description).To(Equal("A preexisting filesystem"))
		})

		Context("#update", func() {

			It("should respond with a 422", func() {
				updateDetailsJson, err := json.Marshal(brokerapi.UpdateDetails{
					ServiceID: "service-id",
				})
				Expect(err).NotTo(HaveOccurred())
				reader := strings.NewReader(string(updateDetailsJson))
				resp, err := httpDoWithAuth("PATCH", "/v2/service_instances/12345", reader)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(422))

				responseBody, err := ioutil.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(responseBody)).To(ContainSubstring("This service does not support instance updates. Please delete your service instance and create a new one with updated configuration."))
			})

		})
	})

	Context("#IsRetired", func() {
		var (
			fakeRetiredStore *fakes.FakeRetiredStore
			retired          bool
			err              error
		)

		JustBeforeEach(func() {
			retired, err = IsRetired(fakeRetiredStore)
		})

		BeforeEach(func() {
			fakeRetiredStore = &fakes.FakeRetiredStore{}
		})

		Context("when the store is not a RetireableStore", func() {
			BeforeEach(func() {
				fakeRetiredStore.IsRetiredReturns(false, nil)
			})

			It("should return false", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(retired).To(BeFalse())
			})
		})

		Context("when the store is a RetiredStore", func() {
			Context("when the store is retired", func() {
				BeforeEach(func() {
					fakeRetiredStore.IsRetiredReturns(true, nil)
				})

				It("should return true", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(retired).To(BeTrue())
				})
			})

			Context("when the store is not retired", func() {
				BeforeEach(func() {
					fakeRetiredStore.IsRetiredReturns(false, nil)
				})

				It("should return false", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(retired).To(BeFalse())
				})
			})

			Context("when the IsRetired check fails", func() {
				BeforeEach(func() {
					fakeRetiredStore.IsRetiredReturns(false, errors.New("is-retired-failed"))
				})

				It("should return true", func() {
					Expect(err).To(MatchError("is-retired-failed"))
				})
			})
		})
	})
})

type failRunner struct {
	Command           *exec.Cmd
	Name              string
	AnsiColorCode     string
	StartCheck        string
	StartCheckTimeout time.Duration
	Cleanup           func()
	session           *gexec.Session
	sessionReady      chan struct{}
}

func (r failRunner) Run(sigChan <-chan os.Signal, ready chan<- struct{}) error {
	defer GinkgoRecover()

	allOutput := gbytes.NewBuffer()

	debugWriter := gexec.NewPrefixedWriter(
		fmt.Sprintf("\x1b[32m[d]\x1b[%s[%s]\x1b[0m ", r.AnsiColorCode, r.Name),
		GinkgoWriter,
	)

	session, err := gexec.Start(
		r.Command,
		gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[32m[o]\x1b[%s[%s]\x1b[0m ", r.AnsiColorCode, r.Name),
			io.MultiWriter(allOutput, GinkgoWriter),
		),
		gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[91m[e]\x1b[%s[%s]\x1b[0m ", r.AnsiColorCode, r.Name),
			io.MultiWriter(allOutput, GinkgoWriter),
		),
	)

	Ω(err).ShouldNot(HaveOccurred())

	fmt.Fprintf(debugWriter, "spawned %s (pid: %d)\n", r.Command.Path, r.Command.Process.Pid)

	r.session = session
	if r.sessionReady != nil {
		close(r.sessionReady)
	}

	startCheckDuration := r.StartCheckTimeout
	if startCheckDuration == 0 {
		startCheckDuration = 5 * time.Second
	}

	var startCheckTimeout <-chan time.Time
	if r.StartCheck != "" {
		startCheckTimeout = time.After(startCheckDuration)
	}

	detectStartCheck := allOutput.Detect(r.StartCheck)

	for {
		select {
		case <-detectStartCheck: // works even with empty string
			allOutput.CancelDetects()
			startCheckTimeout = nil
			detectStartCheck = nil
			close(ready)

		case <-startCheckTimeout:
			// clean up hanging process
			session.Kill().Wait()

			// fail to start
			return fmt.Errorf(
				"did not see %s in command's output within %s. full output:\n\n%s",
				r.StartCheck,
				startCheckDuration,
				string(allOutput.Contents()),
			)

		case signal := <-sigChan:
			session.Signal(signal)

		case <-session.Exited:
			if r.Cleanup != nil {
				r.Cleanup()
			}

			Expect(string(allOutput.Contents())).To(ContainSubstring(r.StartCheck))
			Expect(session.ExitCode()).To(Not(Equal(0)), fmt.Sprintf("Expected process to exit with non-zero, got: 0"))
			return nil
		}
	}
}

type credhubInfoResponse struct {
	AuthServer credhubInfoResponseAuthServer `json:"auth-server"`
}

type credhubInfoResponseAuthServer struct {
	URL string `json:"url"`
}
