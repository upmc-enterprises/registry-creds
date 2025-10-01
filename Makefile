PLATFORM ?= linux/amd64,linux/arm64,linux/arm
PROGRESS ?= auto
SOURCE_DATE_EPOCH := $(shell git log -1 --format=%ct)
COMMIT_ISO := $(shell git log -1 --format=%cI)
REGISTRY ?= ghcr.io/manifest-cyber
IMAGE_NAME ?= registry-creds
BINARY_NAME = registry-creds
VERSION := latest
SHELL := /bin/bash
OUT_DIR ?= out
BIN_DIR ?= $(OUT_DIR)/bins
IMAGE_DIR ?= $(OUT_DIR)/image
MAIN_PATH=./main.go
GO_SRCS=$(shell find . -type f -name "*.go" -not -path "*/\.*")
MOD_SRCS=$(shell find . -type f -name "go.mod" -o -name "go.sum" -not -path "*/\.*")
SRCS=$(GO_SRCS) $(MOD_SRCS)
LICENSE_CODE=AGPL-3.0
SOURCE_URL=https://github.com/manifest-cyber/registry-creds
DESCRIPTION='registry-creds - Auto-update ImagePullSecrets in Kubernetes for common container registries'

export SOURCE_DATE_EPOCH
export TZ=UTC
export LANG=C.UTF-8
export LC_ALL=C
export BUILDKIT_MULTI_PLATFORM=1
export DOCKER_BUILDKIT=1

ifeq ($(NOCACHE), 1)
NOCACHE_FLAG=--no-cache
else
NOCACHE_FLAG=
endif
export NOCACHE_FLAG

all: \
	$(BIN_DIR) \
	image \
	build-linux-amd64 \
	build-linux-arm64 \
	build-linux-arm \
	build-mac-amd64 \
	build-mac-arm64

$(OUT_DIR):
	@mkdir -p $(OUT_DIR)

$(IMAGE_DIR): $(OUT_DIR)
	@mkdir -p $(IMAGE_DIR)

$(BIN_DIR): $(OUT_DIR)
	@mkdir -p $(BIN_DIR)

$(BIN_DIR)/%: $(SRCS) $(OUT_DIR)
	go build \
		$(LDFLAGS) \
		-o $(BIN_DIR)/$* \
		$(MAIN_PATH)

.PHONY: test
test: $(SRCS)
	go test -v ./...

# Build for specific platforms
build-linux-amd64: $(SRCS) | $(BIN_DIR)
	CGO_ENABLED=0 \
	GOOS=linux \
	GOARCH=amd64 \
	$(MAKE) $(BIN_DIR)/$(BINARY_NAME)_linux_amd64

build-linux-arm64: $(SRCS) | $(BIN_DIR)
	CGO_ENABLED=0 \
	GOOS=linux \
	GOARCH=arm64 \
	$(MAKE) $(BIN_DIR)/$(BINARY_NAME)_linux_arm64

build-linux-arm: $(SRCS) | $(BIN_DIR)
	CGO_ENABLED=0 \
	GOOS=linux \
	GOARCH=arm \
	GOARM=7 \
	$(MAKE) $(BIN_DIR)/$(BINARY_NAME)_linux_armv7

build-mac-amd64: $(SRCS) | $(BIN_DIR)
	CGO_ENABLED=0 \
	GOOS=darwin \
	GOARCH=amd64 \
	$(MAKE) $(BIN_DIR)/$(BINARY_NAME)_darwin_amd64 $(MAIN_PATH)

build-mac-arm64: $(SRCS) | $(BIN_DIR)
	CGO_ENABLED=0 \
	GOOS=darwin \
	GOARCH=arm64 \
	$(MAKE) $(BIN_DIR)/$(BINARY_NAME)_darwin_arm64

.PHONY: image
image: $(IMAGE_DIR)/index.json
$(IMAGE_DIR)/index.json: Containerfile LICENSE $(IMAGE_DIR) $(SRCS)
	docker \
		buildx \
		build \
		--ulimit nofile=2048:16384 \
		--tag $(REGISTRY)/$(IMAGE_NAME):$(VERSION) \
		--output \
			name=$(IMAGE_NAME),type=oci,rewrite-timestamp=true,force-compression=true,annotation.org.opencontainers.licenses=$(LICENSE_CODE),annotation.org.opencontainers.image.revision=$(shell git rev-list HEAD -1 .),annotation.org.opencontainers.source=$(SOURCE_URL),annotation.org.opencontainers.image.created=$(COMMIT_ISO),annotation.org.opencontainers.description=$(DESCRIPTION),tar=true,dest=- \
		$(EXTRA_ARGS) \
		$(NOCACHE_FLAG) \
		$(CHECK_FLAG) \
		--platform=$(PLATFORM) \
		--progress=$(PROGRESS) \
		--sbom=true \
		--provenance=true \
		-f $< \
		. \
		| tar -C $(IMAGE_DIR) -mx

.PHONY: load-image
load-image: image
	tar -C $(IMAGE_DIR) -cf - . | docker load

.PHONY: image-digests
.ONESHELL:
image-digests: $(OUT_DIR)/digests.txt
$(OUT_DIR)/digests.txt: $(IMAGE_DIR)/index.json
	@INDEX_DIGEST=$$(jq -r '.manifests[0].digest' $(IMAGE_DIR)/index.json) && \
	MANIFEST_FILE=$$(echo "$$INDEX_DIGEST" | sed 's/sha256://' | xargs -I {} find $(IMAGE_DIR)/blobs/sha256 -name "{}" -type f) && \
	if [ -n "$$MANIFEST_FILE" ]; then \
		jq -r '.manifests[] | select(.annotations."vnd.docker.reference.type" != "attestation-manifest") | "\(.digest | sub("sha256:"; "")) \(.platform.os)/\(.platform.architecture)"' "$$MANIFEST_FILE" | sort > $@; \
	else \
		echo "Error: Could not find manifest file for $$INDEX_DIGEST"; \
		exit 1; \
	fi

.PHONY: show-image-digests
show-image-digests: $(OUT_DIR)/digests.txt
	@cat $<

.PHONY: github-digest-summary
github-digest-summary:
	@echo "## Digest Comparison"
	@echo ""
	@echo "| Digest 1 | Digest 2 | Match | Platform |"
	@echo "|----------|----------|-------|----------|"
	@while IFS=' ' read -r digest1 platform1; do \
		digest2=$$(grep "^[^ ]* $$platform1$$" $(DIGESTS2) | cut -d' ' -f1 2>/dev/null || echo "N/A"); \
		if [ "$$digest1" = "$$digest2" ] && [ "$$digest1" != "N/A" ]; then \
			match="✅"; \
		else \
			match="❌"; \
		fi; \
		printf "| \`%s\` | \`%s\` | %s | %s |\n" "$$digest1" "$$digest2" "$$match" "$$platform1"; \
	done < $(DIGESTS1); \
	while IFS=' ' read -r digest2 platform2; do \
		if ! grep -q "^[^ ]* $$platform2$$" $(DIGESTS1) 2>/dev/null; then \
			printf "| \`N/A\` | \`%s\` | :x: | %s |\n" "$$digest2" "$$platform2"; \
		fi; \
	done < $(DIGESTS2)

.PHONY: verify
verify: all
	@$(MAKE) all OUT_DIR=out2
	@$(MAKE) image-digests
	@$(MAKE) image-digests OUT_DIR=out2
	@cmp $(OUT_DIR)/digests.txt out2/digests.txt
	@cmp $(BIN_DIR)/$(BINARY_NAME)_darwin_arm64 out2/bins/$(BINARY_NAME)_darwin_arm64
	@cmp $(BIN_DIR)/$(BINARY_NAME)_darwin_amd64 out2/bins/$(BINARY_NAME)_darwin_amd64
	@cmp $(BIN_DIR)/$(BINARY_NAME)_linux_armv7 out2/bins/$(BINARY_NAME)_linux_armv7
	@cmp $(BIN_DIR)/$(BINARY_NAME)_linux_arm64 out2/bins/$(BINARY_NAME)_linux_arm64
	@cmp $(BIN_DIR)/$(BINARY_NAME)_linux_amd64 out2/bins/$(BINARY_NAME)_linux_amd64
	@echo "Digests match!"
