package ssadb

import "time"

var (
	_SSASaveTypeCost   uint64
	_SSAVariableCost   uint64
	_SSASourceCodeCost uint64
)

func GetSSASaveTypeCost() time.Duration {
	return time.Duration(_SSASaveTypeCost * uint64(time.Millisecond))
}

func GetSSAVariableCost() time.Duration {
	return time.Duration(_SSAVariableCost * uint64(time.Millisecond))
}

func GetSSASourceCodeCost() time.Duration {
	return time.Duration(_SSASourceCodeCost * uint64(time.Millisecond))
}
