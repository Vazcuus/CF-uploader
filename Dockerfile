# Сборка 
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o s3-uploader ./cmd/api/main.go

# Финальный образ 
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/s3-uploader .

EXPOSE 8080

CMD ["./s3-uploader"]