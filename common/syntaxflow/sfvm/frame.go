package sfvm

type SFFrame[T comparable, V any] struct {
	Text  string
	Codes []*SFI[T, V]
}
