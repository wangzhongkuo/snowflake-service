## This is a self-documented Makefile. For usage information, run `make help`:
##
## For more information, refer to https://suva.sh/posts/well-documented-makefiles/

SHELL = bash

PROJECT_ROOT := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))

GOBIN := $(shell echo $(GOBIN) | cut -d':' -f1)
GOPATH := $(shell echo $(GOPATH) | cut -d':' -f1)

# It defaults to a directory named go inside your home directory, so $HOME/go.
DEFAULT_GOPATH := $(shell go env GOPATH | cut -d: -f1)
BIN := ""

# GOBIN > GOPATH > Default GOPATH ($HOME/go)
ifneq ($(GOBIN),)
	BIN=$(GOBIN)
else 
	ifneq ($(GOPATH),)
		BIN=$(GOPATH)/bin
	else 
		BIN=$(DEFAULT_GOPATH)/bin
	endif
endif

export PATH := $(abspath $(BIN)):$(PATH)

GIT_COMMIT := $(shell git rev-parse HEAD)
GO_LDFLAGS := "-X main.Version=v1.0.0-$(GIT_COMMIT)"

# Figure out which machine we're running on.
THIS_OS := $(shell uname | tr A-Z a-z)
THIS_ARCH := $(shell uname -m | tr A-Z a-z)

default: help

##@ Service

# go source files, ignore api directory
GO_FILES=$(shell find . -iname '*.go' -type f | grep -v /vendor/ |grep -v ".gen.go"| grep -v ".pb.go") # All the .go files, excluding vendor/

APP_BIN_FILE := __service

.PHONY: build
build: ## Build Go binary.
	@go build -ldflags $(GO_LDFLAGS) -o "$(APP_BIN_FILE)"

.PHONY: run
run: build ## Run Go binary.
	@./"$(APP_BIN_FILE)"

.PHONY: fmt
fmt: ## Go fmt
	@gofmt -l -w $(GO_FILES)

.PHONY: test
test: ## Go test
	@go test -v ./... -short

.PHONY: lint
lint: ## Golangci lint
	@$(BIN)/golangci-lint run

.PHONY: clean
clean: ## Clean Go binary.
	@if [ -f $(APP_BIN_FILE) ] ; then rm -f $(APP_BIN_FILE) ; fi

##@ Docker
.PHONY: up
up: ## Docker compose up
	@docker compose up -d

.PHONY: down
down: ## Docker compose down
	@docker compose down

##@ Environment

# Dependency versions
PROTOC_VERSION := 3.19.4
PROTOC_GEN_GO_VERSION := 1.27.1
PROTOC_GEN_GO_GRPC_VERSION := 1.2.0
PROTOC_INSTALLED_DIR := .protoc
PROTOC_ZIP := /tmp/protoc-$(PROTOC_VERSION).zip
PROTO_PATH := ./proto
PROTO_OUTPUT := $(PROTO_PATH)
PROTO_FILES := $(shell find $(PROTO_PATH) -name '*.proto')
PROTOC_BIN := $(PROTOC_INSTALLED_DIR)/bin/protoc
CURRENT_PROTOC_VERSION := $(shell $(PROTOC_BIN) --version 2>&1 | awk '{print $$2}')
PROTOC_DOWNLOAD_URL := https://s3.shiyou.kingsoft.com/software/protobuf/$(PROTOC_VERSION)/protoc-$(PROTOC_VERSION)-$(THIS_OS)-$(THIS_ARCH).zip

.PHONY: setup
setup:protoc protoc-gen-go protoc-gen-go-grpc go-mock go-lint ## Install tools and dependencies.
	@echo "install finished"

#
# Protoc dynamic shared library dependency problem in Arm64
# Download protoc
# Use install_name_tool to change protoc dynamic shared librarty default install path
# Protoc indicates a new signature
#
# otool - view dependency dynamic shared librarty
# otool -L ./bin/protoc 
# ./bin/protoc:
#	@@HOMEBREW_CELLAR@@/protobuf/3.19.4/lib/libprotobuf.30.dylib (compatibility version 31.0.0, current version 31.4.0)
#	@@HOMEBREW_CELLAR@@/protobuf/3.19.4/lib/libprotoc.30.dylib (compatibility version 31.0.0, current version 31.4.0)
# 
# insatll_name_tool - change dynamic shared librarty install names
# install_name_tool -change old new input 
#
# codesign -s - -f ./bin/protoc 
#
# dylib_path : dynamic shared librarty install path
# dylib_name : dynamic shared librarty name
.PHONY: protoc
protoc:
	@if [ $(CURRENT_PROTOC_VERSION) != $(PROTOC_VERSION) ]; then \
		if curl -L $(PROTOC_DOWNLOAD_URL) -o $(PROTOC_ZIP); then \
			rm -rf $(PROTOC_INSTALLED_DIR); \
			unzip -q -o $(PROTOC_ZIP) -d $(PROTOC_INSTALLED_DIR); \
			if [[ $(THIS_OS) == darwin && $(THIS_ARCH) == arm64 ]]; then \
				dylib_paths=`otool -L $(PROTOC_INSTALLED_DIR)/bin/protoc | grep @@HOMEBREW_CELLAR@@ | awk '{print $$1}'`; \
				for dylib_path in $$dylib_paths; do \
					dylib_name=`echo $$dylib_path | awk -F/ '{print $$NF}'`; \
					install_name_tool -change $$dylib_path $(PROTOC_INSTALLED_DIR)/lib/$$dylib_name $(PROTOC_INSTALLED_DIR)/bin/protoc >/dev/null 2>&1 ; \
					dylib_path=`otool -L $(PROTOC_INSTALLED_DIR)/lib/$$dylib_name | grep @@HOMEBREW_CELLAR@@ | awk '{print $$1}'`; \
					codesign -s - -f $(PROTOC_INSTALLED_DIR)/bin/protoc >/dev/null 2>&1; \
					if [ $! $$dylib_path ]; then \
						install_name_tool -change $$dylib_path $(PROTOC_INSTALLED_DIR)/lib/`echo $$dylib_path | awk -F/ '{print $$NF}'` \
						$(PROTOC_INSTALLED_DIR)/lib/$$dylib_name >/dev/null 2>&1; \
						codesign -s - -f $(PROTOC_INSTALLED_DIR)/lib/$$dylib_name >/dev/null 2>&1; \
					fi \
				done \
			fi \
		fi \
	fi

.PHONY: protoc-gen-go
protoc-gen-go:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v$(PROTOC_GEN_GO_VERSION)

.PHONY: protoc-gen-go-grpc
protoc-gen-go-grpc:
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v$(PROTOC_GEN_GO_GRPC_VERSION)

GOLANG_MOCK_VERSION := 1.6.0
.PHONY: go-mock
go-mock:
	go install github.com/golang/mock/mockgen@v$(GOLANG_MOCK_VERSION)

GOLANG_CI_LINTER_VERSION := 1.45.2
.PHONY: go-lint
go-lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v$(GOLANG_CI_LINTER_VERSION)

##@ Code
.PHONY: generate
generate: ## Generate proto files, mock files
	@(PROTOC_BIN) \
		--proto_path=$(PROTOC_INSTALLED_DIR)/include \
		--proto_path=$(PROTO_PATH) \
		--go_out=$(PROTO_OUTPUT) --go_opt=paths=source_relative \
		--go-grpc_out=$(PROTO_OUTPUT) --go-grpc_opt=require_unimplemented_servers=false --go-grpc_opt=paths=source_relative \
		$(PROTO_FILES)
	@echo "Finished."

.PHONY: help
help:
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
