BINARY := terraform-provider-allinkl

.PHONY: build test testacc fmt vet tidy generate clean

build:
	go build -o $(BINARY)

# Regenerate docs/ from the provider schema, examples/ and templates/.
generate:
	go tool tfplugindocs generate --provider-name allinkl --rendered-provider-name allinkl

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
