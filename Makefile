## This is a self-documented Makefile. For usage information, run `make help`:
##
## For more information, refer to https://www.thapaliya.com/en/writings/well-documented-makefiles/

ROOTDIR := $(abspath $(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
DISTDIR := $(abspath $(ROOTDIR)/dist)

BUILD_VERSION := $(shell $(ROOTDIR)/scripts/version)
BUILD_COMMIT := $(shell git rev-parse HEAD^{commit})
BUILD_STAMP := $(shell date -u '+%Y-%m-%d %H:%M:%S+00:00')

ifneq (,)
$(foreach v, \
	$(sort $(.VARIABLES)), \
	$(info ===== $(origin $(v)) $(v) = $($(v))) \
)
endif

RUN_TOOL := $(ROOTDIR)/scripts/docker-run

include config.mk

-include local/Makefile

S := @
V :=

GO := GO111MODULE=on CGO_ENABLED=0 go
GO_VENDOR := $(if $(realpath $(ROOTDIR)/vendor/modules.txt),true,false)
GO_BUILD_COMMON_FLAGS := -trimpath
ifeq ($(GO_VENDOR),true)
	GO_BUILD_MOD_FLAGS := -mod=vendor
	GOLANGCI_LINT_MOD_FLAGS := --modules-download-mode=vendor
else
	GO_BUILD_MOD_FLAGS := -mod=readonly
	GOLANGCI_LINT_MOD_FLAGS := --modules-download-mode=readonly
endif
GO_BUILD_FLAGS := $(GO_BUILD_MOD_FLAGS) $(GO_BUILD_COMMON_FLAGS)

GO_PKGS ?= ./...
SH_FILES ?= $(shell find ./scripts -name *.sh)

GO_TEST_ARGS ?= $(GO_PKGS)

COMMANDS := $(shell test -d cmd && $(GO) list $(GO_BUILD_MOD_FLAGS) ./cmd/...)
EXAMPLES := $(shell test -d examples && $(GO) list $(GO_BUILD_MOD_FLAGS) ./examples/...)

VERSION_PKG := $(shell test -d internal/version && $(GO) list $(GO_BUILD_MOD_FLAGS) ./internal/version)

TEST_OUTPUT := $(DISTDIR)/test

.DEFAULT_GOAL := all

.PHONY: all
all: deps build

##@ Dependencies

.PHONY: deps-go
deps-go: ## Install Go dependencies.
ifeq ($(GO_VENDOR),true)
	$(GO) mod vendor
else
	$(GO) mod download
endif
	$(GO) mod verify
	$(GO) mod tidy -compat=1.17

.PHONY: deps
deps: deps-go ## Install all dependencies.

##@ Building

BUILD_GO_TARGETS := $(addprefix build-go-, $(COMMANDS) $(EXAMPLES))

.PHONY: $(BUILD_GO_TARGETS)
$(BUILD_GO_TARGETS): build-go-%:
	$(call build_go_command,$*)

.PHONY: build-go
build-go: $(BUILD_GO_TARGETS) ## Build all Go binaries.
	$(S) echo Done.

.PHONY: build
build: build-go ## Build everything.

##@ Testing

.PHONY: test-go
test-go: ## Run Go tests.
test-go: export CGO_ENABLED=1 # -race needs CGO_ENABLED
test-go:
	$(S) echo "test backend"
	$(V) mkdir -p $(TEST_OUTPUT)
	$(V) $(RUN_TOOL) gotestsum \
		--format standard-verbose \
		--jsonfile $(TEST_OUTPUT).json \
		--junitfile $(TEST_OUTPUT).xml \
		-- \
		$(GO_BUILD_MOD_FLAGS) \
		-cover \
		-coverprofile=$(TEST_OUTPUT).cov \
		-race \
		$(GO_TEST_ARGS)
	$(S) $(ROOTDIR)/scripts/report-test-coverage $(TEST_OUTPUT).cov

.PHONY: test
test: test-go ## Run all tests.

##@ Linting

.PHONY: golangci-lint
golangci-lint:
	$(S) echo "lint via golangci-lint"
	$(S) $(RUN_TOOL) golangci-lint run \
		$(GOLANGCI_LINT_MOD_FLAGS) \
		--config '$(ROOTDIR)/.golangci.yaml' \
		$(GO_PKGS)

.PHONY: go-vet
go-vet:
	$(S) echo "lint via go vet"
	$(S) $(GO) vet $(GO_BUILD_MOD_FLAGS) $(GO_PKGS)

.PHONY: lint-go
lint-go: go-vet golangci-lint ## Run all Go code checks.

.PHONY: lint
lint: lint-go ## Run all code checks.

##@ Packaging
.PHONY: package
package: build ## Build Debian and RPM packages.
	$(S) echo "Building Debian and RPM packages..."		
	$(S) sh scripts/package/package.sh

.PHONY: publish-packages
publish-packages: package ## Publish Debian and RPM packages to the repository.
	$(S) echo "Publishing Debian and RPM packages...."
	$(S) sh scripts/package/publish.sh
	
##@ Helpers

.PHONY: clean
clean: ## Clean up intermediate build artifacts.
	$(S) echo "Cleaning intermediate build artifacts..."
	$(V) rm -rf node_modules
	$(V) rm -rf public/build
	$(V) rm -rf dist/build
	$(V) rm -rf dist/publish

.PHONY: distclean
distclean: clean ## Clean up all build artifacts.
	$(S) echo "Cleaning all build artifacts..."
	$(V) git clean -Xf

.PHONY: help
help: ## Display this help.
	$(S) awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: docker
docker: build
	$(S) docker build -t $(DOCKER_TAG) ./

.PHONY: docker-push
docker-push:  docker
	$(S) docker push $(DOCKER_TAG)
	$(S) docker tag $(DOCKER_TAG) $(DOCKER_TAG):$(BUILD_VERSION)
	$(S) docker push $(DOCKER_TAG):$(BUILD_VERSION)

define build_go_command
	$(S) echo 'Building $(1)'
	$(S) mkdir -p dist
	$(V) $(GO) build \
		$(GO_BUILD_FLAGS) \
		-o '$(DISTDIR)/$(notdir $(1))' \
		-ldflags '-X "$(VERSION_PKG).commit=$(BUILD_COMMIT)" -X "$(VERSION_PKG).version=$(BUILD_VERSION)" -X "$(VERSION_PKG).buildstamp=$(BUILD_STAMP)"' \
		'$(1)'
endef
