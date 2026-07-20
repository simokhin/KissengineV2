BINARY_NAME := KissengineV2
BUILD_DIR   := bin
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)-$(shell date +%Y%m%d%H%M%S)

.PHONY: build test vet fmt clean run

build:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION) ./cmd

run: build
	./$(BUILD_DIR)/$(BINARY_NAME)-$(VERSION)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

clean:
	rm -rf $(BUILD_DIR)
