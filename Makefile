ROOT := $(CURDIR)
SOURCES := $(shell find . -name "*.go" -or -name "go.mod" -or -name "go.sum" \
	-or -name "Makefile")

# Verbose output
ifdef VERBOSE
V = -v
endif

#
# Environment
#

BINDIR := bin
TOOLDIR := $(BINDIR)/tools

# Global environment variables for all targets
# - go test with race requires CGO
SHELL ?= /bin/bash
SHELL := env \
	GO111MODULE=on \
	GOBIN=$(CURDIR)/$(TOOLDIR) \
	CGO_ENABLED=1 \
	PATH='$(CURDIR)/$(BINDIR):$(CURDIR)/$(TOOLDIR):$(PATH)' \
	$(SHELL)

#
# Defaults
#

# Default target
.DEFAULT_GOAL := build

.PHONY: all
all: lint test build

#
# Tools
#

TOOLS += $(TOOLDIR)/gobin
gobin: $(TOOLDIR)/gobin
$(TOOLDIR)/gobin:
	GO111MODULE=off go get -u github.com/myitcv/gobin

# external tool
define tool # 1: binary-name, 2: go-import-path
TOOLS += $(TOOLDIR)/$(1)

.PHONY: $(1)
$(1): $(TOOLDIR)/$(1)

$(TOOLDIR)/$(1): $(TOOLDIR)/gobin Makefile
	gobin $(V) "$(2)"
endef

# internal tool
define inttool # 1: name
TOOLS += $(TOOLDIR)/$(1)

.PHONY: $(1)
$(1): $(TOOLDIR)/$(1)

$(TOOLDIR)/$(1): $(SOURCES)
	cd "tools/$(1)" && go build $(V) -o "$(ROOT)/$(TOOLDIR)/$(1)"
endef

$(eval $(call tool,genny,github.com/geoah/genny@v1.0.3))
$(eval $(call tool,gofumports,mvdan.cc/gofumpt/gofumports))
$(eval $(call tool,golangci-lint,github.com/golangci/golangci-lint/cmd/golangci-lint@v1.38.0))
$(eval $(call tool,golds,go101.org/golds@v0.2.0))
$(eval $(call tool,mockgen,github.com/golang/mock/mockgen@v1.5.0))
$(eval $(call tool,wwhrd,github.com/frapposelli/wwhrd@v0.4.0))
$(eval $(call tool,golines,github.com/segmentio/golines@v0.1.0))
$(eval $(call tool,go-mod-upgrade,github.com/oligot/go-mod-upgrade@v0.6.1))
$(eval $(call tool,goreleaser,github.com/goreleaser/goreleaser@v0.177.0))

$(eval $(call inttool,codegen))
$(eval $(call inttool,community))

.PHONY: tools
tools: $(TOOLS)

#
# Build
#

MODULE := github.com/geoah/go-rpc
LDFLAGS := -w -s

# Development
#

# Clean up everything
.PHONY: clean
clean:
	rm -f coverage.out coverage.tmp-*.out
	rm -f $(BINS) $(TOOLS) $(EXAMPLES)
	rm -f ./go.mod.tidy-check ./go.sum.tidy-check
	rm -f $(OUTPUT_DIR)

# Tidy go modules
.PHONY: tidy
tidy:
	$(info Tidying go modules)
	@find . -type f -name "go.sum" -not -path "./vendor/*" -execdir rm {} \;
	@find . -type f -name "go.mod" -not -path "./vendor/*" -execdir go mod tidy \;

# Upgrade go modules
.PHONY: upgrade
upgrade: go-mod-upgrade
	@$(TOOLDIR)/go-mod-upgrade
	@make tidy

# Tidy dependecies and make sure go.mod has been committed
# Currently only checks the main go.mod
.PHONY: check-tidy
check-tidy:
	$(info Checking if go.mod is tidy)
	cp go.mod go.mod.tidy-check
	cp go.sum go.sum.tidy-check
	go mod tidy
	( \
		diff go.mod go.mod.tidy-check && \
		diff go.sum go.sum.tidy-check && \
		rm -f go.mod go.sum && \
		mv go.mod.tidy-check go.mod && \
		mv go.sum.tidy-check go.sum \
	) || ( \
		rm -f go.mod go.sum && \
		mv go.mod.tidy-check go.mod && \
		mv go.sum.tidy-check go.sum; \
		exit 1 \
	)

# Install deps
.PHONY: deps
deps:
	$(info Installing dependencies)
	@go mod download

# Run go test
.PHONY: test
test:
	@go test $(V) -count=1 --race ./...

# Run go test -bench
.PHONY: benchmark
benchmark:
	@go test $(V) -run=^$$ -bench=. ./...

# Lint code
.PHONY: lint
lint: golangci-lint
	$(info Running Go linters)
	@GOGC=off golangci-lint $(V) run

# Check licenses
.PHONY: check-licenses
check-licenses: wwhrd
	$(info Checking licenses)
	@go mod vendor
	@wwhrd check
