package cifra

import "time"

// SuppressOvernightStaleAlert recognizes the observed USDT/RUB overnight pause,
// not a guaranteed exchange maintenance schedule. The alert engine only calls
// it after rejecting a stale quote. Quotes last updated at 01:45–02:00 Moscow
// time may stay quiet until 04:00 that same morning (22:45–23:00 and 01:00 UTC).
func SuppressOvernightStaleAlert(quoteTime, now time.Time) bool {
	quote := quoteTime.UTC()
	if quote.IsZero() || quote.Hour() != 22 || quote.Minute() < 45 || !now.After(quote) {
		return false
	}
	// An absolute cutoff prevents yesterday's frozen quote from being muted again.
	cutoff := time.Date(quote.Year(), quote.Month(), quote.Day()+1, 1, 0, 0, 0, time.UTC)
	return now.Before(cutoff)
}
