# Makefile for the Docker image upmcenterprises/registry-creds
# MAINTAINER: Steve Sloka <slokas@upmc.edu>
# If you update this image please bump the tag value before pushing.

TAG = 1.6
PREFIX = upmcenterprises

BIN = registry-creds
ARCH = amd64

# go option
GO      ?= go
LDFLAGS := -w
GOFLAGS := -a -installsuffix cgo

# docker build arguments for internal proxy
ifneq ($(http_proxy),)
HTTP_PROXY_BUILD_ARG=--build-arg http_proxy=$(http_proxy)
else
HTTP_PROXY_BUILD_ARG=
endif

ifneq ($(https_proxy),)
HTTPS_PROXY_BUILD_ARG=--build-arg https_proxy=$(https_proxy)
else
HTTPS_PROXY_BUILD_ARG=
endif

.PHONY: all
all: container

.PHONY: build
build: build-dirs bin/$(BIN) bin/linux_$(ARCH)/$(BIN)

# local developement binary (auto detect developer OS)
bin/$(BIN): main.go
	@echo "Building: $@"
	GOARCH=$(ARCH) CGO_ENABLED=0 $(GO) install
	GOARCH=$(ARCH) CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $@ --ldflags '$(LDFLAGS)' $<

# docker image binary
bin/linux_$(ARCH)/$(BIN): main.go
	@echo "Building: $@"
	GOOS=linux GOARCH=$(ARCH) CGO_ENABLED=0 $(GO) install
	GOOS=linux GOARCH=$(ARCH) CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $@ --ldflags '$(LDFLAGS)' $<

.PHONY: build-dirs
build-dirs:
	@mkdir -p bin/linux_$(ARCH)

.PHONY: container
container: build
	docker build -t $(PREFIX)/$(BIN):$(TAG) \
		$(HTTP_PROXY_BUILD_ARG) \
		$(HTTPS_PROXY_BUILD_ARG) .

.PHONY: push
push:
	docker push $(PREFIX)/$(BIN):$(TAG)

.PHONY: clean
clean:
	rm -rf bin

.PHONY: test
test: clean
	$(GO) test -v $(go list ./... | grep -v vendor)
