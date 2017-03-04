# Makefile for the Docker image upmcenterprises/registry-creds
# MAINTAINER: Steve Sloka <slokas@upmc.edu>
# If you update this image please bump the tag value before pushing.

.PHONY: all binary container push clean test

TAG = 1.6
PREFIX = upmcenterprises

all: container

build: main.go
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -a -installsuffix cgo -o registry-creds --ldflags '-w' ./main.go

container: build
	docker build -t $(PREFIX)/registry-creds:$(TAG) .

push:
	docker push $(PREFIX)/registry-creds:$(TAG)

clean:
	rm -f registry-creds

test: clean
	go test -v $(go list ./... | grep -v vendor)
