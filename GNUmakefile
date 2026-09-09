default: build

BINARY_NAME=terraform-provider-gravitino
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	go build -o bin/$(BINARY_NAME) -ldflags "-X main.version=$(VERSION)" .

test:
	go test -v -cover ./internal/...

testacc:
	TF_ACC=1 go test -v -cover ./internal/...

lint:
	golangci-lint run ./...

lint-fix:
	golangci-lint run --fix ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

install: build
	mkdir -p ~/.terraform.d/plugins/registry.terraform.io/gravitino/gravitino/$(VERSION)/$$(go env GOOS)_$$(go env GOARCH)
	cp bin/$(BINARY_NAME) ~/.terraform.d/plugins/registry.terraform.io/gravitino/gravitino/$(VERSION)/$$(go env GOOS)_$$(go env GOARCH)/$(BINARY_NAME)

testacc-docker:
	docker compose run --rm test

testacc-live:
	podman compose up -d gravitino
	podman compose run --rm acc

testacc-live-filter:
	@test -n "$(F)" || (echo "usage: make testacc-live-filter F=TestLiveAccMetalakeResource"; exit 1)
	podman compose up -d gravitino
	GO_TEST_FILTER="$(F)" podman compose run --rm acc

generate:
	go generate ./...

.PHONY: build test testacc lint lint-fix fmt vet install testacc-docker testacc-live testacc-live-filter generate
