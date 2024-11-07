# Makefile for MySQL Manager

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
BINARY_NAME=mysql-manager
VERSION=$(shell git describe --tags --always --dirty)
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Directories
CMD_DIR=cmd/mysql-manager
INTERNAL_DIR=internal

# Build targets
.PHONY: all build test clean deps vet lint docker

all: test build

build:
	$(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME) $(CMD_DIR)/main.go

test:
	$(GOTEST) -v ./...

vet:
	$(GOCMD) vet ./...

lint:
	golangci-lint run

clean:
	rm -f bin/$(BINARY_NAME)
	$(GOCMD) clean -modcache

deps:
	$(GOMOD) download
	$(GOMOD) tidy

docker:
	docker build -t mysql-manager:$(VERSION) .
	docker-compose up -d

coverage:
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out

# CI/CD targets
ci: deps vet lint test build

# Development targets
dev:
	$(GOBUILD) -race -o bin/$(BINARY_NAME) $(CMD_DIR)/main.go
