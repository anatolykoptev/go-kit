.RECIPEPREFIX = >

.PHONY: test lint cover preflight

test:
> @GOWORK=off go test ./...

lint:
> @GOWORK=off golangci-lint run --allow-parallel-runners ./...

cover:
> @GOWORK=off go test -coverprofile=coverage.out ./...
> @GOWORK=off go tool cover -func=coverage.out

preflight:
> @echo "==> gofmt -l"
> @out=$$(GOWORK=off gofmt -l .); \
> if [ -n "$$out" ]; then \
> echo "FAIL: gofmt drift in the following files (run: gofmt -w <file>):"; \
> echo "$$out"; \
> exit 1; \
> fi
> @echo "==> go vet ./..."
> @GOWORK=off go vet ./...
> @echo "==> go build ./..."
> @GOWORK=off go build ./...
> @echo "==> go test -race -count=1 ./..."
> @GOWORK=off go test -race -count=1 ./...
