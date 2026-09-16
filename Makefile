BINARY := ipblacklist-builder
PACKAGE := ./cmd/ipblacklist-builder

.PHONY: all test vet build release clean

all: test build

test:
	go test ./...

vet:
	go vet ./...

build:
	go build -trimpath -o bin/$(BINARY) $(PACKAGE)

release:
	go test ./...
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/$(BINARY)-linux-amd64 $(PACKAGE)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/$(BINARY)-linux-arm64 $(PACKAGE)

clean:
	go clean
