.PHONY: build build-gui test run clean pack lint

APP_NAME := go-launcher
BUILD_DIR := build
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"
GOFLAGS := -trimpath

build:
	@echo "Building $(APP_NAME) v$(VERSION)..."
	go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)$(shell go env GOEXE) ./cmd/launcher

build-gui:
	@echo "Building $(APP_NAME) v$(VERSION) (hidden console)..."
	go build $(GOFLAGS) $(LDFLAGS) -ldflags="-s -w -H windowsgui -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME).exe ./cmd/launcher

build-all:
	@echo "Cross-compiling..."
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -ldflags="-s -w -H windowsgui -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe ./cmd/launcher
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 ./cmd/launcher
	GOOS=darwin GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 ./cmd/launcher

test:
	@echo "Running tests..."
	go test -v -race -count=1 ./...

test-short:
	@echo "Running short tests..."
	go test -short -count=1 ./...

run:
	@echo "Starting $(APP_NAME)..."
	go run $(GOFLAGS) ./cmd/launcher

lint:
	@echo "Running linters..."
	go vet ./...
	staticcheck ./...

clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	rm -rf dist
	go clean -cache

pack: build
	@echo "Packaging..."
	cp -r web $(BUILD_DIR)/web
	cd $(BUILD_DIR) && zip -r ../$(APP_NAME)-$(VERSION).zip . && cd ..
	@echo "Created $(APP_NAME)-$(VERSION).zip"

pack-all: build-all
	@echo "Packaging all platforms..."
	for f in $(BUILD_DIR)/$(APP_NAME)-*; do \
		ext=$${f##*.}; \
		name=$$(basename $$f); \
		dir=$${name%.*}; \
		mkdir -p $(BUILD_DIR)/$$dir/web; \
		cp $$f $(BUILD_DIR)/$$dir/; \
		cp -r web/* $(BUILD_DIR)/$$dir/web/; \
		cd $(BUILD_DIR)/$$dir && zip -r ../$$dir.zip . && cd ../..; \
	done
	@echo "All packages created."

deps:
	@echo "Installing dependencies..."
	go mod tidy
	go install honnef.co/go/tools/cmd/staticcheck@latest

help:
	@echo "Usage:"
	@echo "  make build      - Build for current platform"
	@echo "  make build-all  - Cross-compile for Win/Linux/macOS"
	@echo "  make test       - Run all tests"
	@echo "  make run        - Run launcher"
	@echo "  make lint       - Run linters"
	@echo "  make clean      - Remove build artifacts"
	@echo "  make pack       - Build + create zip"
	@echo "  make pack-all   - Build all + create zips"
