BINARY := gdoc2md
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)
GOFLAGS := -trimpath

.PHONY: build clean all

build:
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY) .

all: clean
	GOOS=darwin  GOARCH=amd64 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/$(BINARY)-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/$(BINARY)-darwin-arm64 .
	GOOS=linux   GOARCH=amd64 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/$(BINARY)-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/$(BINARY)-linux-arm64 .
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/$(BINARY)-windows-amd64.exe .
	GOOS=windows GOARCH=arm64 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/$(BINARY)-windows-arm64.exe .

clean:
	rm -rf $(BINARY) dist/
