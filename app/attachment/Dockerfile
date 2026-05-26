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
    go build -ldflags="-s -w" -o /app/attachment app/attachment/attachment.go

FROM alpine
RUN apk add --no-cache ca-certificates tzdata
ENV TZ Asia/Shanghai
WORKDIR /app
COPY --from=builder /app/attachment /app/attachment
COPY app/attachment/rpc/etc /app/etc
COPY app/attachment/model/migrations /app/model/migrations
EXPOSE 8091
CMD ["./attachment", "-f", "etc/attachment.yaml"]
