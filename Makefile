BINARY := waitfor
PKG := ./cmd/waitfor

.PHONY: build build-linux build-arm test lint release clean

build:
	go build -o bin/$(BINARY) $(PKG)

build-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/$(BINARY)-linux-amd64 $(PKG)

build-arm:
	GOOS=linux GOARCH=arm64 go build -o bin/$(BINARY)-linux-arm64 $(PKG)

test:
	go test ./...

lint:
	golangci-lint run

release:
	goreleaser release --clean

clean:
	rm -rf bin dist
