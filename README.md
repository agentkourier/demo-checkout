# checkout

The checkout service confirms orders. For each order it asks the payments service to
authorize the amount and confirms the order once payments approves it.

Owned by the checkout team. Part of the `commerce` system.

## API

### `POST /v1/checkout`

Request:

```json
{"order_id": "ord_1001", "amount_cents": 4250, "currency": "USD"}
```

`order_id` is required, `amount_cents` must be positive, and `currency` is an upper-case
ISO 4217 code. Invalid requests get `400` with `{"error": "..."}`.

Checkout calls `POST {PAYMENTS_URL}/v1/authorize` with the same fields and reads
`authorization_id`, `status` and `amount` (in cents) from the response.

| Result                                                          | Status | Body                                                                       |
|-----------------------------------------------------------------|--------|----------------------------------------------------------------------------|
| Payments approved the full amount                               | `200`  | `{"order_id": "ord_1001", "status": "confirmed", "authorization_id": "..."}` |
| Payments declined                                               | `402`  | `{"error": "payment declined"}`                                            |
| Payments unreachable, non-2xx, unreadable response, or amount mismatch | `502`  | `{"error": "payment authorization failed"}`                         |

Every `502` is logged at error level with the cause.

### Operational endpoints

| Endpoint       | Purpose                                            |
|----------------|----------------------------------------------------|
| `GET /healthz` | Liveness: `200` while the process is up.           |
| `GET /readyz`  | Readiness: `200` once serving, `503` after SIGTERM. |
| `GET /metrics` | Prometheus metrics.                                |

## Configuration

| Variable                      | Default  | Description                                                     |
|-------------------------------|----------|-----------------------------------------------------------------|
| `PAYMENTS_URL`                | required | Base URL of the payments service, e.g. `http://payments:8080`.  |
| `ADDR`                        | `:8080`  | Listen address.                                                 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset    | OTLP gRPC endpoint for traces. Tracing is off when it is unset. |

The standard `OTEL_*` exporter and resource variables also apply. Spans carry
`service.name=checkout` and `service.version` set to the build version.

## Telemetry

- Metrics: `http_requests_total{handler,code}`, `http_request_duration_seconds{handler}`,
  and `payments_client_requests_total{outcome}` with outcome `ok`, `http_error` or
  `decode_error`, plus the Go runtime and process collectors.
- Traces: one server span per request (probes and `/metrics` excluded). The payments call
  runs in a `payments.Authorize` client span, which records the error when the call fails or
  its response cannot be decoded; the HTTP request inside it is a client span from
  `otelhttp`, which propagates W3C trace context to payments.
- Logs: JSON on stdout, one line per request; lines logged within a traced request include
  `trace_id` and `span_id`.

## Build and run

```sh
make test                                          # go vet and go test -race
make build                                         # bin/checkout, version from git describe
PAYMENTS_URL=http://localhost:8081 make run        # go run on :8080
make image VERSION=v1.8.0                          # docker image demo-checkout:v1.8.0
```

The version is set at build time with `-ldflags "-X main.version=..."`; the image
carries it in the `org.opencontainers.image.version` label and the commit in
`org.opencontainers.image.revision`.
