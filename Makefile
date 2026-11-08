.PHONY: build test race vet check screenshots clean deps tidy

build:
	go build ./...

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

check: build test vet

screenshots:
	go run ./cmd/render-examples

clean:

# ── Utilities ─────────────────────────────────────────────────────────────────

deps:
	go mod download
	go mod verify

tidy:
	go mod tidy
