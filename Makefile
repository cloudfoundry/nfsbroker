IMAGE_NAME      :=      cfpersi/smb-broker-k8s

all: install

install:
	go install -v

test:
	docker kill nfsbroker-dev || true
	docker rm nfsbroker-dev || true
	docker run --name=nfsbroker-dev -d -v ~/workspace/nfsbroker:/nfsbroker -v ~/workspace/credhub:/credhub --privileged  cfpersi/nfs-broker-tests /bin/bash -c "cd credhub && ./scripts/start_server.sh -Dspring.profiles.active=dev,dev-h2"
	docker exec -it nfsbroker-dev /bin/bash -c "until curl -f -v http://localhost:9001/health; do sleep 10; done && cp credhub/applications/credhub-api/src/test/resources/server_ca_cert.pem /tmp/server_ca_cert.pem && cd nfsbroker && ginkgo -v -r -keepGoing -p -trace -randomizeAllSpecs -progress ."


fmt:
	go fmt ./...

.PHONY: install test fmt 
