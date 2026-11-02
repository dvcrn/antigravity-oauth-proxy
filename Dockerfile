FROM --platform=$BUILDPLATFORM golang:1.25.7-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Cross-compile on the build platform for each target architecture
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -mod=readonly -trimpath -buildvcs=false -ldflags="-s -w -buildid=" \
    -o antigravity-oauth-proxy ./cmd/antigravity-oauth-proxy

FROM alpine:latest

RUN apk --no-cache add ca-certificates

RUN addgroup -g 1001 appgroup && \
    adduser -D -u 1001 -G appgroup appuser

WORKDIR /app

COPY --from=builder /app/antigravity-oauth-proxy .
RUN chown appuser:appgroup antigravity-oauth-proxy

USER appuser

EXPOSE 9878

CMD ["./antigravity-oauth-proxy"]
