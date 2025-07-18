FROM golang:1.24-alpine AS builder

RUN go version

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o rinha-backend-2025 ./cmd/web/*.go

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/rinha-backend-2025 .

EXPOSE 8000

CMD ["./rinha-backend-2025"]