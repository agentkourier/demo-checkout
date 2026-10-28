// Package client holds the HTTP clients for the services checkout calls.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Authorization is the part of the payments authorize response checkout uses.
type Authorization struct {
	ID     string `json:"authorization_id"`
	Status string `json:"status"`

	// Amount is the authorized amount in cents.
	Amount int64 `json:"amount"`
}

// AuthorizeRequest is the body of POST /v1/authorize on payments.
type AuthorizeRequest struct {
	OrderID     string `json:"order_id"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
}

var (
	// ErrHTTP is returned when payments cannot be reached or answers with a
	// non-2xx status.
	ErrHTTP = errors.New("payments request failed")
	// ErrDecode is returned when the payments response body cannot be decoded.
	ErrDecode = errors.New("decode payments response")
)

// Outcomes of an Authorize call, as reported by Outcome.
const (
	OutcomeOK          = "ok"
	OutcomeHTTPError   = "http_error"
	OutcomeDecodeError = "decode_error"
)

// Outcome classifies an error returned by Authorize.
func Outcome(err error) string {
	switch {
	case err == nil:
		return OutcomeOK
	case errors.Is(err, ErrDecode):
		return OutcomeDecodeError
	default:
		return OutcomeHTTPError
	}
}

var tracer = otel.Tracer("github.com/agentkourier/demo-checkout/client")

// maxResponseBytes bounds how much of a payments response is read.
const maxResponseBytes = 1 << 20

// Payments calls the payments service.
type Payments struct {
	baseURL string
	http    *http.Client
}

// NewPayments returns a client for the payments service at baseURL.
func NewPayments(baseURL string, hc *http.Client) *Payments {
	return &Payments{baseURL: strings.TrimSuffix(baseURL, "/"), http: hc}
}

// Authorize asks payments to authorize req. Errors wrap ErrHTTP or ErrDecode.
//
// The call runs in a client span that stays open until the response is
// decoded, so a response that payments sent but checkout cannot read is
// recorded on the trace. (The otelhttp transport's span ends as soon as the
// body has been read, before decoding fails.)
func (p *Payments) Authorize(ctx context.Context, req AuthorizeRequest) (Authorization, error) {
	ctx, span := tracer.Start(ctx, "payments.Authorize",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("order.id", req.OrderID)))
	defer span.End()

	// fail records cause on the span and returns it wrapped in kind.
	fail := func(kind, cause error) (Authorization, error) {
		span.RecordError(cause)
		span.SetStatus(codes.Error, cause.Error())
		return Authorization{}, fmt.Errorf("%w: %w", kind, cause)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return Authorization{}, fmt.Errorf("encode authorize request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/authorize", bytes.NewReader(body))
	if err != nil {
		return fail(ErrHTTP, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return fail(ErrHTTP, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		return fail(ErrHTTP, fmt.Errorf("status %d", resp.StatusCode))
	}

	var auth Authorization
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&auth); err != nil {
		return fail(ErrDecode, err)
	}
	return auth, nil
}
