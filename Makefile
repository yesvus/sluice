.PHONY: check build

check:
	@unformatted=$$(gofmt -l .) || exit 1; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	go vet ./...
	go test -race ./...

build:
	mkdir -p bin
	go build -o bin/sluice ./cmd/sluice
