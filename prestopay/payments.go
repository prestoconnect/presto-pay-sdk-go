package prestopay

// Operation identifies which gateway call an Error came from.
type Operation int

const (
	OpInit Operation = iota
	OpQuery
	OpReverse
	OpRefund
	OpWebhook
	OpConfig
)
