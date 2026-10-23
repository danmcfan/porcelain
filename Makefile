BIN       := bin/porcelain
PKG       := ./cmd/porcelain
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X main.version=$(VERSION)
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64
HOST      := $(shell go env GOOS)/$(shell go env GOARCH)

.PHONY: build test test-short lint fixtures dist skill release publish clean

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

# Per-platform Skill bundles (Skill + that platform's binary) and checksums.
release: dist
	rm -rf dist/release && mkdir -p dist/release
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		d=dist/release/stage-$$os-$$arch/porcelain; mkdir -p $$d/bin; \
		cp skill/porcelain/SKILL.md skill/porcelain/timeline.schema.json $$d/; \
		cp dist/porcelain-$$os-$$arch$$ext $$d/bin/porcelain$$ext; \
		(cd dist/release/stage-$$os-$$arch && zip -qr ../porcelain-skill-$$os-$$arch.zip porcelain) || exit 1; \
		cp dist/porcelain-$$os-$$arch$$ext dist/release/; \
		rm -rf dist/release/stage-$$os-$$arch; \
	done
	cd dist/release && shasum -a 256 porcelain-* > SHA256SUMS
	@ls dist/release

# Publish dist/release as a GitHub Release. Needs a tag: make publish VERSION=v0.1.0
publish: release
	@case "$(VERSION)" in v[0-9]*) ;; *) echo "set VERSION=vX.Y.Z"; exit 1;; esac
	git tag $(VERSION) && git push origin $(VERSION)
	gh release create $(VERSION) dist/release/* --title $(VERSION) --generate-notes

clean:
	rm -rf bin dist testdata/fixtures
