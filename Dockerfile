FROM golang:1.24.0-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o rinha-backend-2025 .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/rinha-backend-2025 .

EXPOSE 8000

CMD ["./rinha-backend-2025"] 