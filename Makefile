BINARY := terraform-provider-allinkl

.PHONY: build test testacc fmt vet tidy generate clean

build:
	go build -o $(BINARY)

# Regenerate docs/ from the provider schema, examples/ and templates/. Uses
# terraform when on PATH, tofu otherwise.
generate:
	./scripts/generate-docs.sh

test:
	go test -race -count=1 ./...

# Acceptance tests hit the real KAS API. Requires KAS_LOGIN / KAS_PASSWORD.
testacc:
	TF_ACC=1 go test -count=1 -timeout 30m ./internal/provider/

fmt:
	gofmt -w .

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -f $(BINARY)
