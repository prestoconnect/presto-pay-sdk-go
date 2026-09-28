package prestopay

import "github.com/prestoconnect/presto-pay-sdk-go/prestopay/internal/canonical"

// Canonicalize renders the canonical string for fields per the gateway's
// signing rule. It is exported for signature debugging: building the exact
// bytes an SDK signed or verified, to compare against a packet capture.
func Canonicalize(fields map[string]any) (string, error) {
	return canonical.Build(fields)
}
