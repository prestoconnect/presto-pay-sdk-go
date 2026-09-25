package prestopay

// Error is implemented by every error type this package returns from a
// network or verification call. See go-plan.md §4 for the concrete types.
type Error interface {
	error
	Operation() Operation
	MayHaveTakenEffect() bool
}
