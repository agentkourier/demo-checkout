package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestTraceLogHandler(t *testing.T) {
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36},
		SpanID:     trace.SpanID{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7},
		TraceFlags: trace.FlagsSampled,
	})

	tests := []struct {
		name      string
		ctx       context.Context
		wantTrace string
		wantSpan  string
	}{
		{"with span", trace.ContextWithSpanContext(context.Background(), sc), "4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7"},
		{"without span", context.Background(), "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(traceLogHandler{slog.NewJSONHandler(&buf, nil)}).With("service", "checkout")
			logger.InfoContext(tt.ctx, "hello")

			var rec map[string]any
			if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
				t.Fatalf("decode log line: %v", err)
			}
			if got, _ := rec["trace_id"].(string); got != tt.wantTrace {
				t.Errorf("trace_id = %q, want %q", got, tt.wantTrace)
			}
			if got, _ := rec["span_id"].(string); got != tt.wantSpan {
				t.Errorf("span_id = %q, want %q", got, tt.wantSpan)
			}
			if rec["service"] != "checkout" {
				t.Errorf("service = %v, want payments", rec["service"])
			}
		})
	}
}
