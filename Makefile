VERSION ?= 0.1.0
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build test clean install release-snapshot

build:
	go build $(LDFLAGS) -o bin/gocode ./cmd/gocode

test:
	go test ./...

clean:
	rm -rf bin/ dist/

install:
	go install $(LDFLAGS) ./cmd/gocode

release-snapshot:
	goreleaser release --snapshot --clean
