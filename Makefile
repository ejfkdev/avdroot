# Build and test avdroot.

BINARY  := avdroot
# The release workflow passes the git tag, so a released binary reports it.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
DIST    := dist

# Every target the release workflow publishes. Patching is pure Go, so these are
# complete binaries with no runtime dependency beyond adb and the emulator.
TARGETS := \
	darwin/amd64 darwin/arm64 \
	linux/amd64 linux/arm64 linux/arm \
	windows/amd64 windows/arm64

.PHONY: all build test vet fmt clean install targets

all: $(addprefix $(DIST)/,$(TARGETS))

# Expand os/arch into an output path, adding .exe for Windows.
$(DIST)/%:
	@mkdir -p $(DIST)
	@os=$(word 1,$(subst /, ,$*)); arch=$(word 2,$(subst /, ,$*)); \
	name=$(BINARY); \
	if [ "$$os" = "windows" ]; then name=$(BINARY).exe; fi; \
	echo "building $$os/$$arch"; \
	GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
		go build -trimpath -ldflags '$(LDFLAGS)' -o "$(DIST)/$(BINARY)_$${os}_$${arch}$$([ "$$os" = windows ] && echo .exe)" .

build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) .

targets:
	@for t in $(TARGETS); do echo $$t; done

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w ./cmd ./internal .

install: build
	install -m 0755 $(BINARY) $(or $(PREFIX),/usr/local/bin)/$(BINARY)

clean:
	rm -rf $(DIST) $(BINARY)
