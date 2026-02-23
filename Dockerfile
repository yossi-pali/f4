FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o f4 ./cmd/server

FROM alpine:3.21
RUN apk --no-cache add ca-certificates tzdata
COPY --from=builder /app/f4 /usr/local/bin/
EXPOSE 8080
CMD ["f4"]
