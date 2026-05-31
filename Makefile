.PHONY: all
all: test build

.PHONY: test
test:
	go test -v ./...

.PHONY: build
build:
	goreleaser build --snapshot --clean

.PHONY: clean
clean:
	rm -rf dist
