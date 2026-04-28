ARG GO_VERSION=1.26.2
ARG BUILD_COMMIT=unknown
ARG BUILD_VERSION=dev

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS builder

ARG TARGETOS
ARG TARGETARCH
ARG BUILD_COMMIT
ARG BUILD_VERSION

WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go test ./...
RUN target_os="${TARGETOS:-linux}" && \
    target_arch="${TARGETARCH:-$(go env GOARCH)}" && \
    version="${BUILD_VERSION:-dev}" && \
    CGO_ENABLED=0 GOOS="${target_os}" GOARCH="${target_arch}" \
    go build -trimpath -ldflags="-s -w -X github.com/michibiki-io/mx-api-go/internal/version.value=${version}" -o /out/mx-api ./cmd/mx-api

FROM gcr.io/distroless/static-debian12:nonroot AS runtime

WORKDIR /app

COPY --from=builder /out/mx-api /app/mx-api
COPY configs/config.example.yaml /etc/mx-api/config.yaml

ENV MX_API_CONFIG=/etc/mx-api/config.yaml

EXPOSE 8080

ENTRYPOINT ["/app/mx-api"]
