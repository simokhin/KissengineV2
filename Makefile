BINARY_NAME := KissengineV2
BUILD_DIR   := bin
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)-$(shell date +%Y%m%d%H%M%S)

.PHONY: build build-windows build-all pgo test vet fmt clean run

# GOAMD64=v3 lets the compiler assume the x86-64-v3 instruction set (POPCNT, BMI,
# AVX2: any Intel since Haswell / AMD since Excavator, 2013-15) instead of
# checking for POPCNT on every bits.OnesCount64; measured ~5% faster. The binary
# won't run on older CPUs. cmd/default.pgo, if present, is used for profile-guided
# optimization automatically (~3%); regenerate it with `make pgo`.
build:
	mkdir -p $(BUILD_DIR)
	GOAMD64=v3 go build -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION) ./cmd

# Cross-compiles a Windows binary (bin/KissengineV2-<version>.exe) from Linux/macOS/WSL;
# same GOAMD64=v3 and PGO as `build`. CGO is off so no C toolchain for Windows is needed.
build-windows:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 GOAMD64=v3 go build -o $(BUILD_DIR)/$(BINARY_NAME)-$(VERSION).exe ./cmd

build-all: build build-windows

# Record a CPU profile of a representative search workload into cmd/default.pgo.
pgo:
	mkdir -p $(BUILD_DIR)
	go test ./engine -run '^$$' -bench BenchmarkPGOProfile -benchtime=1x -cpuprofile $(CURDIR)/cmd/default.pgo -o $(BUILD_DIR)/engine.test

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
