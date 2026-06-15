FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app/dashboard .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/dashboard /dashboard
EXPOSE 8080
CMD ["/dashboard"]
