FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/facility-service ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache tzdata ca-certificates
ENV TZ=Asia/Bangkok
COPY --from=build /out/facility-service /usr/local/bin/facility-service
EXPOSE 8082
ENTRYPOINT ["facility-service"]
