GO ?= go

.PHONY: test test-race update vttest vttest-update bench cover build vet tidy

# Level 1-3: unit, ANSI/integration and headless GPU tests.
test:
	$(GO) test ./...

# Level 4: thread-safety gate. Every test also runs under the race detector.
test-race:
	$(GO) test -race ./...

# Regenerate golden files, then review the diff before committing.
update:
	$(GO) test ./... -update

# Opt-in conformance suite: drives the real vttest(1) binary through the parser
# in a headless PTY (requires vttest in PATH). Skipped by plain `make test`.
vttest:
	VTTEST=1 $(GO) test ./internal/vte -run Vttest -count=1 -v

# Re-record the vttest golden snapshots, then review the diff before committing.
vttest-update:
	VTTEST=1 $(GO) test ./internal/vte -run Vttest -count=1 -update

# Allocation/throughput guards (zero-alloc ring, low-alloc parser).
bench:
	$(GO) test -run=^$$ -bench=. -benchmem ./...

cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

build:
	$(GO) build -o bin/go-vte ./cmd/go-vte

vet:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy
