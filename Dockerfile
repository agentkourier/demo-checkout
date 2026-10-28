# syntax=docker/dockerfile:1

FROM golang:1.27.2 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/checkout .

FROM gcr.io/distroless/static-debian12:nonroot
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.title="checkout" \
      org.opencontainers.image.source="https://github.com/agentkourier/demo-checkout" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"
COPY --from=build /out/checkout /checkout
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/checkout"]
