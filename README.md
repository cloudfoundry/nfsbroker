# nfsbroker
A Cloud Foundry service broker for existing nfsv3 shares.

For details on how to use this broker, please refer to [the nfs-volume-release README](https://github.com/cloudfoundry/nfs-volume-release)

# Running tests

```
docker run --name=nfsbroker-dev -t -i -v ~/workspace/nfsbroker:/nfsbroker -v ~/workspace/credhub:/credhub --privileged  cfpersi/nfs-broker-tests bash

> cd credhub
  ./scripts/start_server.sh -Dspring.profiles.active=dev,dev-h2

Back on the host machine. (In a separate terminal tab)

> docker exec -it nfsbroker-dev bash
> until curl -f -v http://localhost:9001/health; do
      sleep 10;
  done

> cp credhub/applications/credhub-api/src/test/resources/server_ca_cert.pem /tmp/server_ca_cert.pem

> cd nfsbroker
> ginkgo -r -keepGoing -p -trace -randomizeAllSpecs -progress .
```
