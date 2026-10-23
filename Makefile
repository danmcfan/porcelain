BIN       := bin/porcelain
PKG       := ./cmd/porcelain
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X main.version=$(VERSION)
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64
HOST      := $(shell go env GOOS)/$(shell go env GOARCH)

.PHONY: build test test-short lint fixtures dist skill clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) $(PKG)

test:
	go test ./...

test-short:
	go test -short ./...

lint:
	golangci-lint run ./...

fixtures:
	scripts/gen-fixtures.sh testdata/fixtures

dist:
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		echo "dist/porcelain-$$os-$$arch$$ext"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' \
			-o dist/porcelain-$$os-$$arch$$ext $(PKG) || exit 1; \
	done

# Assemble an installable Skill folder for the host platform.
skill: build
	rm -rf dist/skill/porcelain
	mkdir -p dist/skill/porcelain/bin
	cp skill/porcelain/SKILL.md skill/porcelain/timeline.schema.json dist/skill/porcelain/
	cp $(BIN) dist/skill/porcelain/bin/
	@echo "dist/skill/porcelain ($(HOST))"

clean:
	rm -rf bin dist testdata/fixtures
