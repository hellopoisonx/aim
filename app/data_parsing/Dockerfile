# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS builder

ENV CGO_ENABLED=0 \
    GOCACHE=/root/.cache/go-build \
    GOMODCACHE=/go/pkg/mod
RUN apk update --no-cache && apk add --no-cache tzdata
WORKDIR /build
ADD go.mod .
ADD go.sum .
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags="-s -w" -o /app/data_parsing app/data_parsing/data_parsing.go

FROM alpine
RUN apk add --no-cache ca-certificates tzdata
ENV TZ Asia/Shanghai
WORKDIR /app
COPY --from=builder /app/data_parsing /app/data_parsing
COPY app/data_parsing/etc /app/etc
CMD ["./data_parsing", "-f", "etc/data_parsing.yaml"]
