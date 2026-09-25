package prestopay

import "time"

// utc8 is the gateway's fixed offset. It is not a *time.Location from the tz
// database: that database may be absent in a scratch container, and a
// political boundary has no business inside a signature.
var utc8 = time.FixedZone("UTC+8", 8*3600)

// timestampLayout renders exactly three fractional digits, unlike ".999"
// which trims trailing zeros.
const timestampLayout = "20060102150405.000"

// FormatTimestamp renders t in the gateway's wire format, always at the
// fixed UTC+8 offset regardless of t's own zone.
func FormatTimestamp(t time.Time) string {
	return t.In(utc8).Format(timestampLayout)
}

// ParseTimestamp reads a gateway timestamp, returning a time.Time in the
// fixed +08:00 zone. It is the supported way to read a §3.8 date field,
// since only some of them are confirmed and an unparseable date should not
// fail an otherwise authentic response.
func ParseTimestamp(s string) (time.Time, error) {
	return time.ParseInLocation(timestampLayout, s, utc8)
}
