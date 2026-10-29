package cifra

import "time"

// SuppressOvernightStaleAlert allows the observed daily Cifra downtime from
// 01:50 inclusive to 05:10 exclusive Moscow time, regardless of quote timestamp.
func SuppressOvernightStaleAlert(_ time.Time, now time.Time) bool {
	local := now.In(time.FixedZone("MSK", 3*60*60))
	minute := local.Hour()*60 + local.Minute()
	return minute >= 110 && minute < 310
}
