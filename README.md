# nfsbroker
A Cloud Foundry service broker for existing nfsv3 shares.

For details on how to use this broker, please refer to [the nfs-volume-release README](https://github.com/cloudfoundry/nfs-volume-release)

# Running tests

```
docker run -t -i -v ~/workspace/nfsbroker:/nfsbroker -v ~/workspace/credhub:/credhub --privileged  cfpersi/nfs-broker-tests bash

> {
      cd credhub
      ./scripts/start_server.sh -Dspring.profiles.active=dev,dev-h2 > /tmp/start_credhub.log 2>&1
      kill -9 "$(ps --pid $$ -oppid=)"; exit
  }&


> until curl -f -v http://localhost:9001/health; do
      sleep 10;
  done

> cd nfsbroker
> ginkgo -r -keepGoing -p -trace -randomizeAllSpecs -progress .
```