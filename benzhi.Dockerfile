FROM golang:1.23-alpine AS builder
RUN apk add --no-cache bash
WORKDIR /src
COPY go.mod go.sum ./
COPY vendor ./vendor
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
COPY config.example.json ./
RUN go build -mod=vendor -o /shorepower ./cmd/server
ENV GOPROXY=off GOSUMDB=off
EXPOSE 8095
CMD ["/shorepower", "-config", "config.example.json"]
