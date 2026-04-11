CONTAINER_NAME=llt-trade
HEALTH_CHECK_PORT=8080
METRICS_PORT=4014

.PHONY: gqlgen repo convert lint lint-fix build

gqlgen:
	@echo "--> Generating gql"
	@rm -rf pkg/action/generated.go && rm -rf pkg/model/models.go
	@go run github.com/99designs/gqlgen generate

repo:
	@cd pkg/repos && sqlc generate

convert:
	goverter gen ./pkg/converter/...

lint:
	@echo "--> Running linter"
	@golangci-lint run

lint-fix:
	@echo "--> Running linter auto fix"
	@golangci-lint run --fix

build:
	@echo "--> Building server"
	@CGO_ENABLED=1 go build -o ./bin ./cmd/app

