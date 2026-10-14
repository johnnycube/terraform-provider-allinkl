# Task-runner contract (https://fleetlint.org/baseline/#task-runner-contract). `make check` is the definition of green.
SHELL := bash
.SHELLFLAGS := -euo pipefail -c
.ONESHELL:
.DEFAULT_GOAL := check-fast
MAIN_BRANCH ?= main
# Total statement coverage of unit plus acceptance tests (79% at the time of writing).
COVER_MIN   ?= 75
BINARY      := terraform-provider-allinkl
PKGS        := ./...
# Compatible with the provider's MPL-2.0; report a dependency outside the list instead of widening it.
LICENSES    ?= MIT,Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,MPL-2.0
# Each pin carries a `# renovate:` comment so Renovate bumps it (customManagers:makefileVersions).
# renovate: datasource=go depName=github.com/golangci/golangci-lint/v2
GOLANGCI_VERSION    ?= v2.14.0
# renovate: datasource=go depName=golang.org/x/vuln
GOVULNCHECK_VERSION ?= v1.9.0
# renovate: datasource=go depName=github.com/google/go-licenses/v2
GO_LICENSES_VERSION ?= v2.0.1
GO_LICENSES := go run github.com/google/go-licenses/v2@$(GO_LICENSES_VERSION)
# Tools are built with this module's toolchain: golangci-lint refuses code that targets a newer Go than it was built with.
TOOLCHAIN   := $(shell go env GOVERSION)
# Acceptance tests drive a real tofu or terraform binary against the in-process fake KAS; no credentials.
TF_BIN      ?= $(shell command -v tofu || command -v terraform)
TF_HOST     := $(if $(findstring tofu,$(notdir $(TF_BIN))),registry.opentofu.org,registry.terraform.io)
ACC_ENV     := TF_ACC=1 TF_ACC_TERRAFORM_PATH=$(TF_BIN) TF_ACC_PROVIDER_HOST=$(TF_HOST)

.PHONY: help tools fmt lint test testacc cover audit build generate docs-check check-fast check tidy clean

help: ## list targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*##' '{printf "  %-12s %s\n", $$1, $$2}'

tools: ## install the pinned Go tools the gates use (gitleaks and tofu/terraform come from their own installers)
	GOTOOLCHAIN=$(TOOLCHAIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	GOTOOLCHAIN=$(TOOLCHAIN) go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

fmt: ## format in place
	golangci-lint fmt

lint: ## format check + linters + vet
	golangci-lint fmt --diff
	golangci-lint run
	go vet $(PKGS)

test: ## unit tests with the race detector, shuffled
	go test -race -shuffle=on -count=1 $(PKGS)

testacc: ## acceptance tests only (needs tofu or terraform)
	@test -n "$(TF_BIN)" || { echo "need tofu or terraform on PATH"; exit 1; }
	$(ACC_ENV) go test -race -count=1 -timeout 15m ./internal/provider/

cover: ## unit + acceptance tests with coverage; fail below COVER_MIN percent
	@test -n "$(TF_BIN)" || { echo "need tofu or terraform on PATH"; exit 1; }
	mkdir -p coverage
	$(ACC_ENV) go test -count=1 -timeout 15m -coverprofile=coverage/cover.out -coverpkg=./internal/... $(PKGS)
	total=$$(go tool cover -func=coverage/cover.out | awk '/^total:/ {sub("%", "", $$3); print $$3}')
	echo "total coverage: $$total% (minimum $(COVER_MIN)%)"
	awk -v t="$$total" -v m="$(COVER_MIN)" 'BEGIN { exit (t + 0 < m + 0) }' || { echo "coverage below $(COVER_MIN)%"; exit 1; }

audit: ## vulnerabilities, licenses, tidy module graph, secrets
	govulncheck $(PKGS)
	$(GO_LICENSES) check $(PKGS) --allowed_licenses=$(LICENSES)
	go mod tidy -diff
	gitleaks git --no-banner --redact .

build: ## build the provider binary
	CGO_ENABLED=0 go build -trimpath -o $(BINARY)

generate: ## regenerate docs/ from the schema, examples/ and templates/ (terraform when on PATH, tofu otherwise)
	./scripts/generate-docs.sh

docs-check: generate ## fail when docs/ is stale
	git diff --exit-code -- docs/ || { echo "docs/ is stale: run 'make generate' and commit the result"; exit 1; }

check-fast: ## before every commit (< 30 s)
	golangci-lint fmt --diff
	golangci-lint run --new-from-rev=origin/$(MAIN_BRANCH) || golangci-lint run
	go test -short $(PKGS)

check: lint test cover audit build ## definition of green (< 10 min)
	fleetlint check --fail-on error

tidy:
	go mod tidy

clean:
	rm -rf $(BINARY) coverage
