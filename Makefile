.PHONY: ci fmt fix vet test hooks

# Everything the checks run, locally: format, go fix, vet, and the tests with the race detector. A new
# check is a new target added to this list. The pre-commit hook runs this (see hooks).
ci: fmt fix vet test

fmt:
	@test -z "$$(gofmt -l server)" || { echo "gofmt needed on:"; gofmt -l server; exit 1; }

# Fails, and prints the patch, when go fix would change a file; apply it with: go fix ./...
fix:
	go fix -diff ./...

vet:
	go vet ./...

test:
	go test -race -count=1 ./...

# Makes git run .githooks/ (the pre-commit hook runs make ci). Once for each clone.
hooks:
	git config core.hooksPath .githooks
