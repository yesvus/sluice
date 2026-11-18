.PHONY: check build

check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	go vet ./...
	go test -race ./...

build:
	go build -o bin/sluice ./cmd/sluice
