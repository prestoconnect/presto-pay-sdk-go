package prestopay

import "time"

// utc8 is the gateway's fixed offset. It is not a *time.Location from the tz
// database: that database may be absent in a scratch container, and a
// political boundary has no business inside a signature.
var utc8 = time.FixedZone("UTC+8", 8*3600)

// timestampLayout renders exactly three fractional digits, unlike ".999"
// which trims trailing zeros.
const timestampLayout = "20060102150405.000"
