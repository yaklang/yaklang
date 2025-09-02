package hnsw

import (
	"math"
	"reflect"
)

// DistanceFunc is a function that computes the distance between two vectors.
type DistanceFunc func(a, b Vector) float64

// EuclideanDistance computes the Euclidean distance between two vectors.
func EuclideanDistance(af, bf Vector) float64 {
	a := af()
	b := bf()
	// TODO: can we speedup with vek?
	var sum float64 = 0
	for i := range a {
		diff := a[i] - b[i]
		sum += float64(diff) * float64(diff)
	}
	return math.Sqrt(sum)
}

var distanceFuncs = map[string]DistanceFunc{
	"euclidean": EuclideanDistance,
	"cosine":    CosineDistance,
}

func distanceFuncToName(fn DistanceFunc) (string, bool) {
	for name, f := range distanceFuncs {
		fnptr := reflect.ValueOf(fn).Pointer()
		fptr := reflect.ValueOf(f).Pointer()
		if fptr == fnptr {
			return name, true
		}
	}
	return "", false
}

// RegisterDistanceFunc registers a distance function with a name.
// A distance function must be registered here before a graph can be
// exported and imported.
func RegisterDistanceFunc(name string, fn DistanceFunc) {
	distanceFuncs[name] = fn
}

// GetDistanceFunc returns the distance function with the given name.
func GetDistanceFunc(name string) DistanceFunc {
	return distanceFuncs[name]
}
