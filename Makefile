# Stint Makefile
#
# CLI and collector packaging targets. `make stint` builds the unified CLI,
# `make collect` builds the collector helper, and install targets copy them
# onto PATH.

GO      ?= go
BIN_DIR ?= bin
COLLECT_BIN := $(BIN_DIR)/stint-collect
STINT_BIN := $(BIN_DIR)/stint
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unset)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
STINT_LDFLAGS := -X github.com/keithah/stint/internal/stintcli.versionValue=$(VERSION) -X github.com/keithah/stint/internal/stintcli.commitValue=$(COMMIT) -X github.com/keithah/stint/internal/stintcli.buildDateValue=$(BUILD_DATE)

# Install location: $GOBIN if set, else $GOPATH/bin if set, else ~/.local/bin.
GOBIN_DIR := $(shell $(GO) env GOBIN)
GOPATH_DIR := $(shell $(GO) env GOPATH)
ifneq ($(strip $(GOBIN_DIR)),)
INSTALL_DIR ?= $(GOBIN_DIR)
else ifneq ($(strip $(GOPATH_DIR)),)
INSTALL_DIR ?= $(GOPATH_DIR)/bin
else
INSTALL_DIR ?= $(HOME)/.local/bin
endif

.PHONY: stint stint-install stint-vet collect collect-install collect-vet vet

## stint: build the unified Stint CLI into ./bin/stint
stint:
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags '$(STINT_LDFLAGS)' -o $(STINT_BIN) ./cmd/stint/
	@echo "built $(STINT_BIN)"

## stint-install: build and install stint plus the collector helper onto PATH ($GOBIN/~/.local/bin)
stint-install: stint collect
	@mkdir -p $(INSTALL_DIR)
	@install -m 0755 $(STINT_BIN) $(INSTALL_DIR)/stint
	@install -m 0755 $(COLLECT_BIN) $(INSTALL_DIR)/stint-collect
	@echo "installed $(INSTALL_DIR)/stint"
	@echo "installed $(INSTALL_DIR)/stint-collect"

## stint-vet: vet the unified Stint CLI command
stint-vet:
	$(GO) vet ./cmd/stint/ ./internal/stintcli/

## collect: build the collector binary into ./bin/stint-collect
collect:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(COLLECT_BIN) ./cmd/collect/
	@echo "built $(COLLECT_BIN)"

## collect-install: build and install stint-collect onto PATH ($GOBIN/~/.local/bin)
collect-install: collect
	@mkdir -p $(INSTALL_DIR)
	@install -m 0755 $(COLLECT_BIN) $(INSTALL_DIR)/stint-collect
	@echo "installed $(INSTALL_DIR)/stint-collect"

## collect-vet: vet the collector command
collect-vet:
	$(GO) vet ./cmd/collect/

## vet: vet CLI and collector commands
vet: stint-vet collect-vet
