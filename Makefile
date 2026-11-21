.PHONY: ci fmt vet test

# Everything the checks run, locally: format, vet, and the tests with the race detector. A new
# check is a new target added to this list.
ci: fmt vet test

fmt:
	@test -z "$$(gofmt -l server)" || { echo "gofmt needed on:"; gofmt -l server; exit 1; }

vet:
	go vet ./...

test:
	go test -race -count=1 ./...
