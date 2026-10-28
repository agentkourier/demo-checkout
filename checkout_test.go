package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/agentkourier/demo-checkout/client"
)

func TestCheckout(t *testing.T) {
	const order = `{"order_id":"ord_1001","amount_cents":4250,"currency":"USD"}`

	tests := []struct {
		name         string
		body         string
		paymentsCode int
		paymentsBody string
		wantCode     int
		wantBody     map[string]any
		wantLog      string
	}{
		{
			name:         "confirmed",
			body:         order,
			paymentsCode: http.StatusOK,
			paymentsBody: `{"authorization_id":"auth_1","order_id":"ord_1001","status":"approved","amount":4250,"currency":"USD"}`,
			wantCode:     http.StatusOK,
			wantBody:     map[string]any{"order_id": "ord_1001", "status": "confirmed", "authorization_id": "auth_1"},
		},
		{
			name:         "payments amount not decodable",
			body:         order,
			paymentsCode: http.StatusOK,
			paymentsBody: `{"authorization_id":"auth_1","order_id":"ord_1001","status":"approved","amount":"42.50","currency":"USD"}`,
			wantCode:     http.StatusBadGateway,
			wantBody:     map[string]any{"error": "payment authorization failed"},
			wantLog:      "cannot unmarshal string",
		},
		{
			name:         "payments unavailable",
			body:         order,
			paymentsCode: http.StatusServiceUnavailable,
			paymentsBody: `{"error":"unavailable"}`,
			wantCode:     http.StatusBadGateway,
			wantBody:     map[string]any{"error": "payment authorization failed"},
			wantLog:      "status 503",
		},
		{
			name:         "declined",
			body:         order,
			paymentsCode: http.StatusOK,
			paymentsBody: `{"authorization_id":"auth_1","order_id":"ord_1001","status":"declined","amount":4250,"currency":"USD"}`,
			wantCode:     http.StatusPaymentRequired,
			wantBody:     map[string]any{"error": "payment declined"},
		},
		{
			name:         "authorized amount mismatch",
			body:         order,
			paymentsCode: http.StatusOK,
			paymentsBody: `{"authorization_id":"auth_1","order_id":"ord_1001","status":"approved","amount":425,"currency":"USD"}`,
			wantCode:     http.StatusBadGateway,
			wantBody:     map[string]any{"error": "payment authorization failed"},
			wantLog:      "authorized amount does not match order",
		},
		{
			name:     "invalid request",
			body:     `{"order_id":"ord_1001","amount_cents":-1,"currency":"USD"}`,
			wantCode: http.StatusBadRequest,
			wantBody: map[string]any{"error": "amount_cents must be positive"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payments := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.paymentsCode)
				_, _ = w.Write([]byte(tt.paymentsBody))
			}))
			defer payments.Close()

			var logs bytes.Buffer
			router := newTestRouter(&logs, client.NewPayments(payments.URL, payments.Client()))

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/checkout", strings.NewReader(tt.body)))

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			var got map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if !reflect.DeepEqual(got, tt.wantBody) {
				t.Errorf("body = %v, want %v", got, tt.wantBody)
			}
			if tt.wantLog != "" {
				if !strings.Contains(logs.String(), tt.wantLog) || !strings.Contains(logs.String(), `"level":"ERROR"`) {
					t.Errorf("logs = %s, want an ERROR line containing %q", logs.String(), tt.wantLog)
				}
			}
		})
	}
}

func newTestRouter(logs *bytes.Buffer, payments authorizer) http.Handler {
	var ready atomic.Bool
	ready.Store(true)
	return newRouter(slog.New(slog.NewJSONHandler(logs, nil)), prometheus.NewRegistry(), payments, &ready)
}
