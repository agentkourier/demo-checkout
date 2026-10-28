package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestAuthorizeDecodeErrorIsRecordedOnClientSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(noop.NewTracerProvider())
		_ = tp.Shutdown(context.Background())
	})

	var traceparent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceparent = r.Header.Get("traceparent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authorization_id":"auth_1","status":"approved","amount":"42.50"}`))
	}))
	defer srv.Close()

	hc := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
	_, err := NewPayments(srv.URL, hc).Authorize(context.Background(), AuthorizeRequest{OrderID: "ord_1", AmountCents: 4250, Currency: "USD"})
	if err == nil {
		t.Fatal("Authorize succeeded, want a decode error")
	}

	var authorize, transport sdktrace.ReadOnlySpan
	for _, s := range recorder.Ended() {
		switch s.Name() {
		case "payments.Authorize":
			authorize = s
		default:
			transport = s
		}
	}
	if authorize == nil || transport == nil {
		t.Fatalf("got %d spans, want payments.Authorize and the HTTP transport span", len(recorder.Ended()))
	}

	if authorize.SpanKind() != trace.SpanKindClient {
		t.Errorf("payments.Authorize kind = %v, want client", authorize.SpanKind())
	}
	if authorize.Status().Code != codes.Error || !strings.Contains(authorize.Status().Description, "cannot unmarshal string") {
		t.Errorf("payments.Authorize status = %+v, want Error with the decode error", authorize.Status())
	}
	var recorded bool
	for _, e := range authorize.Events() {
		if e.Name == "exception" {
			for _, a := range e.Attributes {
				if a.Key == "exception.message" && strings.Contains(a.Value.AsString(), "cannot unmarshal string") {
					recorded = true
				}
			}
		}
	}
	if !recorded {
		t.Errorf("payments.Authorize has no exception event with the decode error: %+v", authorize.Events())
	}

	if transport.SpanKind() != trace.SpanKindClient || transport.Parent().SpanID() != authorize.SpanContext().SpanID() {
		t.Errorf("transport span kind %v parent %v, want a client child of payments.Authorize", transport.SpanKind(), transport.Parent().SpanID())
	}
	if want := transport.SpanContext().TraceID().String(); !strings.Contains(traceparent, want) {
		t.Errorf("payments received traceparent %q, want trace %s", traceparent, want)
	}
}
