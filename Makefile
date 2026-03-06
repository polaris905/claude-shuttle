BINARY_NAME=claude-shuttle
BUILD_DIR=dist
SRC=./cmd/claude-shuttle
VERSION ?= 0.1.0-alpha
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.buildDate=$(BUILD_DATE)

.PHONY: build all clean

# Build for current platform
build:
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME).exe $(SRC)

# Cross-compile for all platforms
all: windows linux darwin

windows:
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(SRC)

linux:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(SRC)

darwin:
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(SRC)

clean:
	rm -rf $(BUILD_DIR)
