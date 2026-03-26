EXECUTABLE := terraform-provider-valsoperator

build:
	go build -o $(EXECUTABLE)

buildnrun:
	go build -o $(EXECUTABLE)
	rm terraform.tfstate
	terraform apply -auto-approve

test:
	go test -v -cover ./...

testacc:
	TF_ACC=1 go test -v -cover -timeout 10m ./...

lint:
	golangci-lint run

docs:
	go generate ./...

fmt:
	gofmt -s -w .

.PHONY: build buildnrun test testacc lint docs fmt
