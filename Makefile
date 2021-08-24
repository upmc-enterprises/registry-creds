# Makefile for the Docker image upmcenterprises/registry-creds
# MAINTAINER: Steve Sloka <steve@stevesloka.com>
# If you update this image please bump the tag value before pushing.

TAG = 1.11
PREFIX = upmcenterprises

BIN = registry-creds

GO111MODULE=off
GO_BINARY=go

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
build: main.go
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO_BINARY) build -a -installsuffix cgo -o $(BIN) --ldflags '-w' $<

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
	rm -f $(BIN)

.PHONY: test
test: clean
	$(GO_BINARY) test -v $(go list ./... | grep -v vendor)
