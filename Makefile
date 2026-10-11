.PHONY: build test clean

build:
	go build ./...

test:
	go test ./...

clean:

# ── Utilities ─────────────────────────────────────────────────────────────────

deps:
	go mod download
	go mod verify

tidy:
	go mod tidy