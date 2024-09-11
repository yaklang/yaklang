package fuzzx

type NoResultError struct{}

func (e *NoResultError) Error() string {
	return "empty result for fuzz request"
}
