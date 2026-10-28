package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agentkourier/demo-checkout/client"
)

func TestMetricsCountPaymentsOutcomes(t *testing.T) {
	responses := []string{
		`{"authorization_id":"auth_1","status":"approved","amount":4250}`,
		`{"authorization_id":"auth_2","status":"approved","amount":"42.50"}`,
		`{"authorization_id":"auth_3","status":"approved","amount":"42.50"}`,
	}
	var n int
	payments := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(responses[n]))
		n++
	}))
	defer payments.Close()

	var logs bytes.Buffer
	router := newTestRouter(&logs, client.NewPayments(payments.URL, payments.Client()))
	for range responses {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/checkout",
			strings.NewReader(`{"order_id":"ord_1","amount_cents":4250,"currency":"USD"}`)))
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	out, _ := io.ReadAll(rec.Body)

	for _, want := range []string{
		`payments_client_requests_total{outcome="ok"} 1`,
		`payments_client_requests_total{outcome="decode_error"} 2`,
		`payments_client_requests_total{outcome="http_error"} 0`,
		`http_requests_total{code="200",handler="/v1/checkout"} 1`,
		`http_requests_total{code="502",handler="/v1/checkout"} 2`,
		`http_request_duration_seconds_count{handler="/v1/checkout"} 3`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("metrics output missing %q", want)
		}
	}
}
