# ---------- ขั้นที่ 1: build ----------
FROM golang:1.26-alpine AS build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o notification-service .

# ---------- ขั้นที่ 2: run ----------
FROM alpine:3.22
WORKDIR /app

COPY --from=build /app/notification-service .

EXPOSE 8084
CMD ["./notification-service"]
