.PHONY: test check

test:
	go test ./...

check:
	go run ./cmd/check
