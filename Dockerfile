FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /server cmd/server/main.go

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /server .
COPY data/ ./data/
COPY internal/db/migrations/ ./internal/db/migrations/

EXPOSE 8080

CMD ["./server"]
