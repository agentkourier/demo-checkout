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
func (p *Payments) Authorize(ctx context.Context, req AuthorizeRequest) (Authorization, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return Authorization{}, fmt.Errorf("encode authorize request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/authorize", bytes.NewReader(body))
	if err != nil {
		return Authorization{}, fmt.Errorf("%w: %w", ErrHTTP, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return Authorization{}, fmt.Errorf("%w: %w", ErrHTTP, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		return Authorization{}, fmt.Errorf("%w: status %d", ErrHTTP, resp.StatusCode)
	}

	var auth Authorization
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&auth); err != nil {
		return Authorization{}, fmt.Errorf("%w: %w", ErrDecode, err)
	}
	return auth, nil
}
