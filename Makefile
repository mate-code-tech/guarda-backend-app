.PHONY: run build migrate

run:
	go run cmd/server/main.go

build:
	go build -o bin/server cmd/server/main.go

migrate:
	go run cmd/server/main.go -migrate
