.PHONY: build clean fmt fmt-check vet test check hooks

build:
	go build -o rdq ./cmd/rdq/

clean:
	rm -f rdq

# fmt rewrites every Go file in the repo with gofmt. Run this after
# touching struct literals or anywhere alignment matters — or just run
# it unconditionally before committing.
fmt:
	gofmt -w .

# fmt-check is the non-mutating counterpart used by CI and the
# pre-commit hook. It exits non-zero and prints the offending paths
# when any file would be rewritten by gofmt.
fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "The following files are not gofmt-formatted:"; \
		echo "$$unformatted"; \
		echo "Run 'make fmt' to fix."; \
		exit 1; \
	fi

vet:
	go vet ./...

test:
	go test -race -count=1 ./...

# check mirrors the CI "Go" job so developers can reproduce any
# red build locally with a single command.
check: fmt-check vet test

# hooks installs the repo's tracked git hooks by pointing
# core.hooksPath at .githooks/. Run once after cloning. Idempotent.
hooks:
	git config core.hooksPath .githooks
	@echo "git hooks installed (core.hooksPath = .githooks)"
