// Package collection contains only the small collection operations used by SCA
// during parser migration. It has no reflection, IO, global state or dependencies.
package collection

func Map[T, R any](in []T, f func(T, int) R) []R {
	out := make([]R, len(in))
	for i, v := range in {
		out[i] = f(v, i)
	}
	return out
}
func FilterMap[T, R any](in []T, f func(T, int) (R, bool)) []R {
	out := make([]R, 0, len(in))
	for i, v := range in {
		if r, ok := f(v, i); ok {
			out = append(out, r)
		}
	}
	return out
}
func Find[T any](in []T, f func(T) bool) (T, bool) {
	for _, v := range in {
		if f(v) {
			return v, true
		}
	}
	var zero T
	return zero, false
}
func MapToSlice[K comparable, V, R any](in map[K]V, f func(K, V) R) []R {
	out := make([]R, 0, len(in))
	for k, v := range in {
		out = append(out, f(k, v))
	}
	return out
}
func SliceToMap[T any, K comparable, V any](in []T, f func(T) (K, V)) map[K]V {
	out := make(map[K]V, len(in))
	for _, v := range in {
		k, r := f(v)
		out[k] = r
	}
	return out
}
func UniqBy[T any, K comparable](in []T, f func(T) K) []T {
	seen := map[K]bool{}
	out := make([]T, 0, len(in))
	for _, v := range in {
		k := f(v)
		if !seen[k] {
			seen[k] = true
			out = append(out, v)
		}
	}
	return out
}
func Assign[K comparable, V any](in ...map[K]V) map[K]V {
	out := map[K]V{}
	for _, m := range in {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}
func PickBy[K comparable, V any](in map[K]V, f func(K, V) bool) map[K]V {
	out := map[K]V{}
	for k, v := range in {
		if f(k, v) {
			out[k] = v
		}
	}
	return out
}
func Values[K comparable, V any](in map[K]V) []V {
	out := make([]V, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	return out
}
func ForEach[T any](in []T, f func(T, int)) {
	for i, v := range in {
		f(v, i)
	}
}
func Clone[K comparable, V any](in map[K]V) map[K]V {
	if in == nil {
		return nil
	}
	out := make(map[K]V, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func MapEntries[K comparable, V any, RK comparable, RV any](in map[K]V, f func(K, V) (RK, RV)) map[RK]RV {
	out := make(map[RK]RV, len(in))
	for k, v := range in {
		rk, rv := f(k, v)
		out[rk] = rv
	}
	return out
}
