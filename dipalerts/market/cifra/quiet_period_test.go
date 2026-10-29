package cifra

import (
	"testing"
	"time"
)

func TestOvernightStaleAlertWindow(t *testing.T) {
	tests := []struct {
		name, quote, checked string
		want                 bool
	}{
		{"before start", "0001-01-01T00:00:00Z", "2026-10-06T01:49:59+03:00", false},
		{"at start", "0001-01-01T00:00:00Z", "2026-10-06T01:50:00+03:00", true},
		{"before end", "2026-10-01T00:00:00Z", "2026-10-06T05:09:59+03:00", true},
		{"at end", "2026-10-01T00:00:00Z", "2026-10-06T05:10:00+03:00", false},
		{"UTC across midnight", "0001-01-01T00:00:00Z", "2026-10-05T23:00:00Z", true},
		{"daytime", "0001-01-01T00:00:00Z", "2026-10-06T12:00:00+03:00", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quote, err := time.Parse(time.RFC3339, tt.quote)
			if err != nil {
				t.Fatal(err)
			}
			now, err := time.Parse(time.RFC3339, tt.checked)
			if err != nil {
				t.Fatal(err)
			}
			if got := SuppressOvernightStaleAlert(quote, now); got != tt.want {
				t.Fatalf("suppressed=%t, want %t", got, tt.want)
			}
		})
	}
}
