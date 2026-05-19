# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS builder

LABEL stage=gobuilder

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
    go build -ldflags="-s -w" -o /app/auth app/auth/rpc/auth.go


FROM alpine

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/share/zoneinfo/Asia/Shanghai /usr/share/zoneinfo/Asia/Shanghai
ENV TZ Asia/Shanghai

WORKDIR /app
COPY --from=builder /app/auth /app/auth
COPY app/auth/rpc/etc /app/etc

EXPOSE 8989

CMD ["./auth", "-f", "etc/auth.yaml"]
