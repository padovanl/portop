.PHONY: build test e2e lint release clean

build:
	go build -o bin/portop ./cmd/portop

test:
	go test ./...

e2e:
	go test -tags=e2e ./e2e/...

lint:
	go vet ./...
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "gofmt needs to be run on:"; \
		gofmt -l .; \
		exit 1; \
	fi

release:
	goreleaser release --snapshot --clean

clean:
	rm -rf bin dist
