.PHONY: test vet fmt build run

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w cmd api config middleware internal model pkg

build:
	../build.sh

run:
	go run ./cmd/api
