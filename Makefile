# Stint Makefile
#
# Collector packaging targets. `make collect` builds the local collector
# binary; `make collect-install` installs it onto PATH.

GO      ?= go
BIN_DIR ?= bin
COLLECT_BIN := $(BIN_DIR)/stint-collect

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

.PHONY: collect collect-install collect-vet

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
