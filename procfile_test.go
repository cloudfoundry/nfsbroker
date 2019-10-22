package main_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
)

var _ = Describe("Procfile", func() {
	var session *gexec.Session
	var command *exec.Cmd

	BeforeEach(func(){
		compiledPath, err := gexec.Build("code.cloudfoundry.org/nfsbroker")
		Expect(err).NotTo(HaveOccurred())
		println(compiledPath)

		_ = os.Mkdir("bin", os.ModePerm)
		Expect("bin").To(BeADirectory())

		brokerBinary, err := os.Open(compiledPath)
		Expect(err).NotTo(HaveOccurred())
		defer brokerBinary.Close()

		copiedFile, err := os.Create("bin/nfsbroker")
		Expect(err).NotTo(HaveOccurred())
		defer copiedFile.Close()

		err = os.Chmod("bin/nfsbroker", os.ModePerm)
		Expect(err).NotTo(HaveOccurred())

		_, err = io.Copy(copiedFile, brokerBinary)
		Expect(err).NotTo(HaveOccurred())

		procFile, err := os.Open("Procfile")
		Expect(err).NotTo(HaveOccurred())

		procFileContents, err := ioutil.ReadAll(procFile)
		Expect(err).NotTo(HaveOccurred())

		procfile := map[string]interface{}{}
		err = yaml.Unmarshal(procFileContents, procfile)
		Expect(err).NotTo(HaveOccurred())

		println(procfile["web"].(string))
		command = exec.Command("bin/nfsbroker",
			"-credhubURL",
			"https://localhost:9000",
			"-credhubCACertPath",
			"/tmp/server_ca_cert.pem",
			"--servicesConfig",
			"default_services.json")
	})

	AfterEach(func(){
		session.Kill()
	})

	It("runs successfully", func(){
		var err error
		session, err = gexec.Start(command, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		Eventually(session).Should(gbytes.Say("nfsbroker.started"))
	})
})
