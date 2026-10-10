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
		{"reported October 5 pause", "2026-10-05T22:53:54Z", "2026-10-06T00:15:00Z", true},
		{"reported October 6 pause", "2026-10-06T22:46:57Z", "2026-10-07T00:15:00Z", true},
		{"reported October 7 pause", "2026-10-07T22:55:05Z", "2026-10-08T00:15:00Z", true},
		{"quote at window start", "2026-10-05T22:45:00Z", "2026-10-06T00:00:00Z", true},
		{"just before cutoff", "2026-10-05T22:53:54Z", "2026-10-06T00:59:59Z", true},
		{"at cutoff", "2026-10-05T22:53:54Z", "2026-10-06T01:00:00Z", false},
		{"daytime", "2026-10-05T22:53:54Z", "2026-10-06T09:00:00Z", false},
		{"previous night must not repeat", "2026-10-05T22:53:54Z", "2026-10-07T00:00:00Z", false},
		{"quote before window", "2026-10-05T22:44:59Z", "2026-10-06T00:00:00Z", false},
		{"quote at window end", "2026-10-05T23:00:00Z", "2026-10-06T00:00:00Z", false},
		{"daytime quote at night", "2026-10-05T18:00:00Z", "2026-10-06T00:00:00Z", false},
		{"future quote", "2026-10-05T22:53:54Z", "2026-10-05T22:45:00Z", false},
		{"missing quote timestamp", "0001-01-01T00:00:00Z", "2026-10-06T00:00:00Z", false},
		{"Moscow offset", "2026-10-06T01:53:54+03:00", "2026-10-06T03:00:00+03:00", true},
		{"year rollover", "2026-12-31T22:53:54Z", "2027-01-01T00:15:00Z", true},
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
