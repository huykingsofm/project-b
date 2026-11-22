.PHONY: ci fmt fix vet lint test hooks

# Everything the checks run, locally: format, go fix, vet, lint, and the tests with the race detector. A new
# check is a new target added to this list. The pre-commit hook runs this (see hooks).
ci: fmt fix vet lint test

fmt:
	@test -z "$$(gofmt -l server platform shared)" || { echo "gofmt needed on:"; gofmt -l server platform shared; exit 1; }

# Fails, and prints the patch, when go fix would change a file; apply it with: go fix ./...
fix:
	go fix -diff ./...

vet:
	go vet ./...

# Lines of at most 100 columns (lll, set in .golangci.yml); a tab counts as 4.
lint:
	golangci-lint run ./...

test:
	go test -race -count=1 ./...

# Makes git run .githooks/ (the pre-commit hook runs make ci). Once for each clone.
hooks:
	git config core.hooksPath .githooks
