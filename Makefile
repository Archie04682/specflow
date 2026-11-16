GO ?= go
PNPM ?= pnpm
BINARY ?= specflow
FRONTEND := frontend
EMBED_DIST := backend/web/dist

# Build identity is injected with -ldflags because the Go toolchain records no
# version-control settings for a `go run` binary, and the development server is
# exactly the build that must be able to say what it is running. The module path
# is read from go.mod so a module rename cannot silently strand the stamps.
VERSION ?=
MODULE := $(shell awk '/^module[[:space:]]/ {print $$2; exit}' go.mod 2>/dev/null)
BUILDINFO := $(MODULE)/backend/internal/buildinfo
REVISION := $(shell git rev-parse HEAD 2>/dev/null)
REVISION_TIME := $(shell git show -s --format=%cI HEAD 2>/dev/null)
# Tracked changes only: run journals and in-flight docs are untracked by design
# and must not make a clean source tree look modified.
MODIFIED := $(shell if git rev-parse --git-dir >/dev/null 2>&1; then if [ -n "$$(git status --porcelain --untracked-files=no)" ]; then echo true; else echo false; fi; fi)
BUILT_AT := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X $(BUILDINFO).version=$(VERSION) -X $(BUILDINFO).revision=$(REVISION) -X $(BUILDINFO).revisionTime=$(REVISION_TIME) -X $(BUILDINFO).modified=$(MODIFIED) -X $(BUILDINFO).builtAt=$(BUILT_AT)

.PHONY: all install dev frontend-build build run test clean

all: build

# Install both toolchains' dependencies. --frozen-lockfile keeps a fresh clone
# honest: if package.json and pnpm-lock.yaml disagree, it fails instead of
# silently changing the lockfile.
install:
	cd $(FRONTEND) && $(PNPM) install --frozen-lockfile
	$(GO) mod download

# Two processes, one terminal: the API on :8080 and Vite on :5173 proxying
# /api to it. `go run` compiles once, so restart this after Go changes; the
# shell warns from /api/server when the running build no longer matches the
# checkout. The trap tears both down when the recipe is interrupted.
dev:
	@trap 'kill $$(jobs -p) 2>/dev/null' INT TERM EXIT; \
	$(GO) run -ldflags "$(LDFLAGS)" ./backend/cmd/api --port 8080 & \
	(cd $(FRONTEND) && $(PNPM) dev) & \
	wait

frontend-build:
	cd $(FRONTEND) && $(PNPM) build

# The frontend must be built first: //go:embed all:dist fails without it, and
# Vite writes straight into that directory (see frontend/vite.config.ts).
build: install frontend-build
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY) ./backend/cmd/api

run: build
	./$(BINARY)

test: install
	$(GO) test ./...
	cd $(FRONTEND) && $(PNPM) exec tsc --noEmit
	cd $(FRONTEND) && $(PNPM) test

clean:
	rm -f $(BINARY)
	find $(EMBED_DIST) -mindepth 1 ! -name .gitkeep -delete
