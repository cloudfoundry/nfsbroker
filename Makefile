all: install

install:
	go install -v

test:
	ginkgo -v -r -keepGoing -p -trace -randomizeAllSpecs -progress .

fmt:
	go fmt ./...

.PHONY: install test fmt 
