// Package canonical builds the canonical string a request or response body
// is signed and verified over.
package canonical

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Build renders the canonical string for fields per the gateway's signing
// rule: every key except "signature", sorted by code point, joined with ":".
// Values must be string, bool, nil, or an integer (any Go integer type or
// json.Number that parses as one) — anything else cannot occur on the wire
// and is rejected rather than guessed at.
func Build(fields map[string]any) (string, error) {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if k == "signature" {
			continue
		}
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(':')
		}
		rendered, err := renderValue(fields[k])
		if err != nil {
			return "", fmt.Errorf("prestopay: field %q: %w", k, err)
		}
		b.WriteString(rendered)
	}
	return b.String(), nil
}

func renderValue(v any) (string, error) {
	switch t := v.(type) {
	case nil:
		return "", nil
	case string:
		return t, nil
	case bool:
		return strconv.FormatBool(t), nil
	case json.Number:
		n, err := strconv.ParseInt(t.String(), 10, 64)
		if err != nil {
			return "", fmt.Errorf("not an integer within int64 range: %q", t.String())
		}
		return strconv.FormatInt(n, 10), nil
	case int:
		return strconv.Itoa(t), nil
	case int32:
		return strconv.FormatInt(int64(t), 10), nil
	case int64:
		return strconv.FormatInt(t, 10), nil
	default:
		return "", fmt.Errorf("value of type %T cannot occur in a signed field", v)
	}
}
