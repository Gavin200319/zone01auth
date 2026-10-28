.PHONY: tidy run test fmt up

tidy:
	go mod tidy

run:
	set -a && . ./.env && set +a && go run ./cmd/server

test:
	go test ./...

fmt:
	gofmt -w .

up:
	docker compose up --build
