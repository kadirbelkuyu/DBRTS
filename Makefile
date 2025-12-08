.PHONY: build clean test test-unit fmt lint deps run run-transfer run-backup run-restore run-list run-explore tools-postgres tools-mongo help

APP_NAME := dbrts
BIN_DIR := bin
BIN_PATH := $(BIN_DIR)/$(APP_NAME)
CONFIG_DIR := configs
SRC_CONFIG ?= $(CONFIG_DIR)/source-mongo.yaml
DST_CONFIG ?= $(CONFIG_DIR)/test-mongo.yaml
EXPLORE_CONFIG ?= $(SRC_CONFIG)

build:
	@echo ">> building $(APP_NAME)"
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_PATH) ./cmd/dbrts

clean:
	@echo ">> cleaning build artifacts"
	@rm -rf $(BIN_DIR)
	@go clean

deps:
	@echo ">> syncing go modules"
	@go mod tidy

fmt:
	@echo ">> formatting go files"
	@gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

lint:
	@echo ">> running go vet"
	@go vet ./...

test:
	@echo ">> running entire test suite"
	@go test ./...

desktop:
	@echo ">> building desktop app"
	@go run ./cmd/dbrts desktop

test-unit:
	@echo ">> running unit tests"
	@go test ./tests/... ./pkg/... ./internal/...

run: build
	@$(BIN_PATH) interactive

run-transfer: build
	@$(BIN_PATH) transfer --source-config $(SRC_CONFIG) --target-config $(DST_CONFIG) --verbose

run-backup: build
	@$(BIN_PATH) backup --config $(SRC_CONFIG) --verbose

run-restore: build
	@$(BIN_PATH) restore --config $(DST_CONFIG) --verbose

run-list: build
	@$(BIN_PATH) list-databases --config $(SRC_CONFIG)

run-explore: build
	@$(BIN_PATH) explore --config $(EXPLORE_CONFIG)

tools-postgres:
	@./scripts/install-postgresql-tools.sh

tools-mongo:
	@./scripts/install-mongodb-tools.sh

help:
	@echo "DBRTS Make targets:"
	@echo "  build          Build the CLI binary"
	@echo "  clean          Remove build artifacts"
	@echo "  deps           Run go mod tidy"
	@echo "  fmt            gofmt all Go files"
	@echo "  lint           Run go vet"
	@echo "  test           go test ./..."
	@echo "  test-unit      Run unit tests only"
	@echo "  run            Launch the interactive CLI"
	@echo "  run-transfer   Run a scripted transfer using configs"
	@echo "  run-backup     Run the backup command"
	@echo "  run-restore    Run the restore command"
	@echo "  run-list       List databases using Source config"
	@echo "  run-explore    Launch the schema explorer"
	@echo "  tools-postgres Install PostgreSQL client tools"
	@echo "  tools-mongo    Install MongoDB Database Tools"
