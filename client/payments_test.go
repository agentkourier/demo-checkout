package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthorize(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		want       Authorization
		wantErr    error
		wantErrMsg string
	}{
		{
			name:   "amount in cents",
			status: http.StatusOK,
			body:   `{"authorization_id":"auth_1","order_id":"ord_1","status":"approved","amount":4250,"currency":"USD"}`,
			want:   Authorization{ID: "auth_1", Status: "approved", Amount: 4250},
		},
		{
			name:       "amount as decimal string",
			status:     http.StatusOK,
			body:       `{"authorization_id":"auth_1","order_id":"ord_1","status":"approved","amount":"42.50","currency":"USD"}`,
			wantErr:    ErrDecode,
			wantErrMsg: "cannot unmarshal string",
		},
		{
			name:    "malformed body",
			status:  http.StatusOK,
			body:    `{"authorization_id":`,
			wantErr: ErrDecode,
		},
		{
			name:    "server error",
			status:  http.StatusInternalServerError,
			body:    `{"error":"boom"}`,
			wantErr: ErrHTTP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotReq AuthorizeRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/authorize" {
					t.Errorf("request = %s %s, want POST /v1/authorize", r.Method, r.URL.Path)
				}
				if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
					t.Errorf("decode request: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			req := AuthorizeRequest{OrderID: "ord_1", AmountCents: 4250, Currency: "USD"}
			got, err := NewPayments(srv.URL+"/", srv.Client()).Authorize(context.Background(), req)

			if gotReq != req {
				t.Errorf("payments received %+v, want %+v", gotReq, req)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("err = %q, want it to contain %q", err, tt.wantErrMsg)
			}
			if got != tt.want {
				t.Errorf("Authorize() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAuthorizeUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	_, err := NewPayments(url, http.DefaultClient).Authorize(context.Background(), AuthorizeRequest{OrderID: "ord_1"})
	if !errors.Is(err, ErrHTTP) {
		t.Fatalf("err = %v, want ErrHTTP", err)
	}
}
