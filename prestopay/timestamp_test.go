package prestopay

import (
	"testing"
	"time"
)

func TestFormatTimestamp_WorkedExample(t *testing.T) {
	tm := time.Date(2025, 4, 23, 10, 45, 0, 0, utc8)
	if got, want := FormatTimestamp(tm), "20250423104500.000"; got != want {
		t.Fatalf("FormatTimestamp() = %q, want %q", got, want)
	}
}

func TestFormatTimestamp_ConvertsFromOtherZones(t *testing.T) {
	utc := time.Date(2025, 4, 23, 2, 45, 0, 0, time.UTC) // 10:45 at UTC+8
	if got, want := FormatTimestamp(utc), "20250423104500.000"; got != want {
		t.Fatalf("FormatTimestamp() = %q, want %q", got, want)
	}
}

func TestParseTimestamp_RoundTrip(t *testing.T) {
	const s = "20260924133756.056"
	tm, err := ParseTimestamp(s)
	if err != nil {
		t.Fatalf("ParseTimestamp: %v", err)
	}
	if got := FormatTimestamp(tm); got != s {
		t.Fatalf("round trip = %q, want %q", got, s)
	}
	if _, offset := tm.Zone(); offset != 8*3600 {
		t.Fatalf("zone offset = %d, want %d", offset, 8*3600)
	}
}

func TestParseTimestamp_RejectsMalformed(t *testing.T) {
	for _, s := range []string{"", "not-a-timestamp", "2025-04-23T10:45:00Z", "20250423104500"} {
		if _, err := ParseTimestamp(s); err == nil {
			t.Fatalf("ParseTimestamp(%q) succeeded, want an error", s)
		}
	}
}

func TestFormatTimestamp_ThreeFractionalDigitsAlwaysShown(t *testing.T) {
	// Go's ".999" trims trailing zeros; ".000" must not, since the wire
	// format always carries exactly three fractional digits.
	tm := time.Date(2025, 1, 1, 0, 0, 0, 0, utc8)
	if got, want := FormatTimestamp(tm), "20250101000000.000"; got != want {
		t.Fatalf("FormatTimestamp() = %q, want %q", got, want)
	}
}
