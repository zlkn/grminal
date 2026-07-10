GO ?= go

.PHONY: test test-race update bench cover build vet tidy

# Level 1-3: unit, ANSI/integration and headless GPU tests.
test:
	$(GO) test ./...

# Level 4: thread-safety gate. Every test also runs under the race detector.
test-race:
	$(GO) test -race ./...

# Regenerate golden files, then review the diff before committing.
update:
	$(GO) test ./... -update

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
