.PHONY: build clean test

BINARY_NAME=mywant-mcp-server

build:
	go build -o $(BINARY_NAME) .

clean:
	rm -f $(BINARY_NAME)

test:
	go test ./...

install: build
	cp $(BINARY_NAME) $(GOPATH)/bin/

.DEFAULT_GOAL := build
