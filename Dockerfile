FROM golang:1.20.3 as build

WORKDIR /app

COPY go.* /app/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=ssh \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build $GO_ARGS -o /app/firescroll

# Need glibc
FROM gcr.io/distroless/base

ENTRYPOINT ["/app/firescroll"]
COPY --from=build /app/firescroll /app/