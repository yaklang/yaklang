package state

import "errors"

var (
	ErrClosed          = errors.New("suricata state store is closed")
	ErrCapacity        = errors.New("suricata state capacity exceeded")
	ErrPerFlowLimit    = errors.New("suricata per-flow state limit exceeded")
	ErrNameTooLong     = errors.New("suricata state name is too long")
	ErrInvalidName     = errors.New("suricata state name is empty")
	ErrInvalidKey      = errors.New("suricata state key is invalid")
	ErrInvalidArgument = errors.New("suricata state argument is invalid")
	ErrOverflow        = errors.New("suricata flow integer overflow")
	ErrSpecMismatch    = errors.New("suricata threshold specification changed within an epoch")
)
