package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/agentkourier/demo-checkout/client"
)

// checkoutRequest is the body of POST /v1/checkout.
type checkoutRequest struct {
	OrderID     string `json:"order_id"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
}

// checkoutResponse is the body returned for a confirmed order.
type checkoutResponse struct {
	OrderID         string `json:"order_id"`
	Status          string `json:"status"`
	AuthorizationID string `json:"authorization_id"`
}

var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

func (r checkoutRequest) validate() error {
	switch {
	case r.OrderID == "":
		return errors.New("order_id is required")
	case r.AmountCents <= 0:
		return errors.New("amount_cents must be positive")
	case !currencyCode.MatchString(r.Currency):
		return errors.New("currency must be an ISO 4217 code")
	}
	return nil
}

// authorizer authorizes payments; *client.Payments implements it.
type authorizer interface {
	Authorize(ctx context.Context, req client.AuthorizeRequest) (client.Authorization, error)
}

type checkoutHandler struct {
	logger   *slog.Logger
	payments authorizer
	// outcomes counts Authorize calls by client.Outcome.
	outcomes *prometheus.CounterVec
}

func (h *checkoutHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req checkoutRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	auth, err := h.payments.Authorize(ctx, client.AuthorizeRequest{
		OrderID:     req.OrderID,
		AmountCents: req.AmountCents,
		Currency:    req.Currency,
	})
	h.outcomes.WithLabelValues(client.Outcome(err)).Inc()
	if err != nil {
		h.logger.ErrorContext(ctx, "payment authorization failed",
			"order_id", req.OrderID, "error", err.Error())
		writeError(w, http.StatusBadGateway, "payment authorization failed")
		return
	}
	if auth.Status != "approved" {
		h.logger.InfoContext(ctx, "payment declined",
			"order_id", req.OrderID, "payment_status", auth.Status)
		writeError(w, http.StatusPaymentRequired, "payment declined")
		return
	}
	if auth.Amount != req.AmountCents {
		h.logger.ErrorContext(ctx, "authorized amount does not match order",
			"order_id", req.OrderID, "amount_cents", req.AmountCents, "authorized_cents", auth.Amount)
		writeError(w, http.StatusBadGateway, "payment authorization failed")
		return
	}

	h.logger.InfoContext(ctx, "order confirmed",
		"order_id", req.OrderID, "authorization_id", auth.ID)
	writeJSON(w, http.StatusOK, checkoutResponse{
		OrderID:         req.OrderID,
		Status:          "confirmed",
		AuthorizationID: auth.ID,
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
