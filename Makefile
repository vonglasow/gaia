# gaia — build helpers and the gates a commit has to pass.

BIN     ?= bin/gaia
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse HEAD 2>/dev/null || echo none)
BUILT   ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
# Where the stamps live. Note this is NOT the path .goreleaser.yaml writes to.
STAMPS   = gaia/plugins/version

# Minimum statement coverage. Raise it deliberately; never lower it for a commit.
COVERAGE_MIN ?= 70
# What each package owes alone. Below the total, since one may honestly sit under it.
PACKAGE_COVERAGE_MIN ?= 45

LDFLAGS = -X $(STAMPS).Version=$(VERSION) -X $(STAMPS).Commit=$(COMMIT) -X $(STAMPS).Date=$(BUILT)

# go install puts tools in GOPATH/bin, which is not always on PATH.
GREMLINS ?= $(shell command -v gremlins || echo "$$(go env GOPATH)/bin/gremlins")

.PHONY: build run test bdd lint cover cover-check tested-check mutation check clean

# A half-written profile is newer than its sources, so a failed recipe must not keep it.
.DELETE_ON_ERROR:

## build: build into ./bin, stamped with version, commit and time
build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) .

## run: run gaia with ARGS, e.g. `make run ARGS="plugins list"`
run:
	go run -ldflags "$(LDFLAGS)" . $(ARGS)

## test: run the whole suite, unit and acceptance alike
test:
	go test ./...

## bdd: run only the acceptance scenarios in bdd/
bdd:
	go test -count=1 ./bdd/...

## lint: the same golangci-lint CI runs, configured in .golangci.yml
lint:
	golangci-lint run ./...

# -coverpkg is what makes bdd/ count for the code it drives: 36.5% without it, 45.9% with.
coverage.out: $(shell find . -name '*.go' -not -path './bin/*') $(wildcard bdd/*.feature)
	@go test ./... -covermode=atomic -coverpkg=./... -coverprofile=$@

## cover: print coverage per function, then the total
cover: coverage.out
	go tool cover -func=coverage.out

## cover-check: fail below the floor, total and per package (root skipped: main only)
cover-check: coverage.out
	@failed=0; \
	total=$$(go tool cover -func=coverage.out | awk '/^total:/ {gsub("%","",$$NF); print $$NF}'); \
	awk -v t="$$total" -v m="$(COVERAGE_MIN)" 'BEGIN { \
		if (t + 0 < m + 0) { printf "\xe2\x9c\x97 coverage %s%% is below the %s%% floor\n", t, m; exit 1 } \
		printf "\xe2\x9c\x93 coverage %s%% \xe2\x89\xa5 %s%%\n", t, m }' || failed=1; \
	awk -v m="$(PACKAGE_COVERAGE_MIN)" ' \
		NR > 1 { \
			block[$$1] = $$2; if ($$3 + 0 > hits[$$1] + 0) hits[$$1] = $$3 } \
		END { \
			for (b in block) { \
				split(b, where, ":"); pkg = where[1]; sub("/[^/]*$$", "", pkg); \
				statements[pkg] += block[b]; if (hits[b] > 0) reached[pkg] += block[b] } \
			for (pkg in statements) { \
				if (pkg == "gaia") continue; \
				share = 100 * reached[pkg] / statements[pkg]; \
				if (share + 0 < m + 0) { \
					printf "\xe2\x9c\x97 %s is at %.1f%%, below the %s%% each package owes\n", pkg, share, m; \
					bad = 1 } } \
			if (bad) exit 1; \
			printf "\xe2\x9c\x93 every package is at %s%% or better\n", m }' coverage.out || failed=1; \
	if [ $$failed -eq 1 ]; then exit 1; fi

## tested-check: fail if a package has no test file at all
tested-check:
	@missing=0; \
	for pkg in $$(go list ./... | grep -v '^gaia$$' | sed "s|^gaia|.|"); do \
		ls $$pkg/*_test.go >/dev/null 2>&1 && continue; \
		echo "✗ $$pkg has no test file"; \
		missing=1; \
	done; \
	if [ $$missing -eq 1 ]; then exit 1; fi; \
	echo "✓ every package has a test file"

## mutation: break each line on purpose and check a test turns red
## Not a commit hook: a pass recompiles the package once per mutant. Pass PKG to
## narrow it, e.g. `make mutation PKG=./plugins/shared/execpolicy`.
mutation:
	@command -v $(GREMLINS) >/dev/null 2>&1 || \
		{ echo "gremlins is not installed: go install github.com/go-gremlins/gremlins/cmd/gremlins@latest"; exit 1; }
	$(GREMLINS) unleash $(PKG)

## check: everything a commit must pass, cheapest first
check: tested-check lint cover-check

## clean: remove build output and the coverage profile
clean:
	rm -rf bin coverage.out
