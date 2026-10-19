default: build

BINARY_NAME=terraform-provider-gravitino
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
# Gravitino server version the acceptance tests run against; docker-compose.yml and the live
# precheck both read it. Override on the command line to test 1.3.1.
GRAVITINO_VERSION ?= 1.3.0
# Supported versions, single-sourced with the CI matrix in .github/workflows/test.yml.
GRAVITINO_VERSIONS := $(shell tr '\n' ' ' < .github/gravitino-versions.txt)

build:
	go build -o bin/$(BINARY_NAME) -ldflags "-X main.version=$(VERSION)" .

test:
	go test -v -cover ./internal/...

# Unit coverage only: `TestAcc*` needs TF_ACC and `TestLiveAcc*` needs a reachable
# server, so both are skipped to keep the profile independent of the environment.
coverage:
	go test -count=1 -skip '^(TestAcc|TestLiveAcc)' -covermode=atomic -coverprofile=coverage.out ./internal/...
	@echo
	@./scripts/coverage-summary.sh coverage.out

coverage-html: coverage
	go tool cover -html=coverage.out

# Unit coverage (coverage.out) plus the acceptance profile written by
# docker-compose.yml (coverage-acceptance.out) as one number.
coverage-merge:
	./scripts/merge-coverage.sh coverage-union.out coverage.out coverage-acceptance.out
	@echo
	@./scripts/coverage-summary.sh coverage-union.out

# Regenerate the coverage badge and block in README.md from
# .github/coverage-baseline.txt; `make coverage-readme-check` is what CI runs to
# fail when the committed README has drifted from the baseline.
coverage-readme:
	./scripts/coverage-readme.sh

coverage-readme-check:
	./scripts/coverage-readme.sh --check

testacc:
	TF_ACC=1 go test -v -cover ./internal/...

lint:
	golangci-lint run --config .github/golangci.yml ./...

lint-fix:
	golangci-lint run --config .github/golangci.yml --fix ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

install: build
	mkdir -p ~/.terraform.d/plugins/registry.terraform.io/caberdo/gravitino/$(VERSION)/$$(go env GOOS)_$$(go env GOARCH)
	cp bin/$(BINARY_NAME) ~/.terraform.d/plugins/registry.terraform.io/caberdo/gravitino/$(VERSION)/$$(go env GOOS)_$$(go env GOARCH)/$(BINARY_NAME)

testacc-docker:
	GRAVITINO_VERSION=$(GRAVITINO_VERSION) docker compose run --rm test

# Run the docker-based acceptance suite against every supported Gravitino server version.
# Each version runs and tears down independently so one failure still lets the others run;
# the overall exit status is non-zero if any version failed.
testacc-matrix:
	@status=0; \
	for v in $(GRAVITINO_VERSIONS); do \
		echo "==> Acceptance tests against Gravitino $$v"; \
		$(MAKE) testacc-docker GRAVITINO_VERSION=$$v || status=1; \
		docker compose down || true; \
	done; \
	exit $$status

testacc-live:
	GRAVITINO_VERSION=$(GRAVITINO_VERSION) podman compose up -d gravitino
	GRAVITINO_VERSION=$(GRAVITINO_VERSION) podman compose run --rm acc

testacc-live-filter:
	@test -n "$(F)" || (echo "usage: make testacc-live-filter F=TestLiveAccMetalakeResource"; exit 1)
	GRAVITINO_VERSION=$(GRAVITINO_VERSION) podman compose up -d gravitino
	GRAVITINO_VERSION=$(GRAVITINO_VERSION) podman compose run --rm -e GO_TEST_FILTER="$(F)" acc

validate-examples:
	./scripts/validate-examples.sh

generate:
	go generate ./...

.PHONY: build test coverage coverage-html coverage-merge coverage-readme coverage-readme-check testacc lint lint-fix fmt vet install testacc-docker testacc-matrix testacc-live testacc-live-filter validate-examples generate
