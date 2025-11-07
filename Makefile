all: lint test
PHONY: test lint golint clean vendor unit-test

test: | vendor lint unit-test vulncheck

serve: | vendor
	@go run main.go serve --config config.yaml

unit-test:
	@echo Running unit tests...
	@go test -cover -short -tags testtools ./...

lint:
	golangci-lint run

vulncheck:
	@echo Running vulnerability check...
	@govulncheck ./...
	@trivy fs --config .trivy.yaml .

build:
	@go mod download
	@CGO_ENABLED=0 GOOS=linux go build -mod=readonly -v -o CHANGEME
	@docker compose build --no-cache

clean: docker-clean
	@echo Cleaning...
	@rm -rf ./dist/
	@rm -rf coverage.out
	@rm -f CHANGEME
	@go clean -testcache

vendor:
	@go mod download
	@go mod tidy

# gen-models-erd:
# 	mermerd -c "postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}_gen?sslmode=disable" \
# 		-e -s 'public' --ignoreTables 'goose_db_version' --useAllTables -o docs/gen_models_erd.md \
# 		--showAllConstraints  --showDescriptions notNull,enumValues,columnComments

# gen-api: | check-oapi-codegen
# 	@go generate ./...

# # go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
# check-oapi-codegen:
# 	@command -v oapi-codegen >/dev/null 2>&1 || { \
# 		echo >&2 "oapi-codegen is required but it's not installed.  Aborting."; exit 1; \
# 	}