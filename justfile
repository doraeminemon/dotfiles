default:
    @just --list

test:
    go test ./...

lint:
    golangci-lint run

vet:
    go vet ./...

build:
    go build -o dist/dot ./cmd/dot

dry-run:
    go run ./cmd/dot install --dry-run --yes

verify: vet test lint
