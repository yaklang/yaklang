//go:build cgo && !hnsw_purego

package hnswspec

/*
#cgo CFLAGS: -O3 -ffast-math
#include <stddef.h>

static double hnsw_dot_f32(const float *a, const float *b, size_t n) {
    double sum = 0.0;
    for (size_t i = 0; i < n; ++i) {
        sum += (double)a[i] * (double)b[i];
    }
    return sum;
}

// hnsw_dot_f32_x8 amortizes the cgo transition across up to eight graph
// vectors. The pointers are individual arguments so the call complies with
// cgo's pointer rules; C neither retains them nor receives Go memory that
// contains Go pointers.
static void hnsw_dot_f32_x8(
    const float *query, size_t n, size_t count,
    const float *v0, const float *v1, const float *v2, const float *v3,
    const float *v4, const float *v5, const float *v6, const float *v7,
    double *out) {
	// Full batches are the common HNSW case. Traversing all eight vectors in
	// one loop lets the compiler interleave their independent accumulators and
	// reads the query only once while retaining double-precision accumulators.
	if (count == 8) {
		double s0 = 0.0, s1 = 0.0, s2 = 0.0, s3 = 0.0;
		double s4 = 0.0, s5 = 0.0, s6 = 0.0, s7 = 0.0;
		for (size_t i = 0; i < n; ++i) {
			double q = (double)query[i];
			s0 += q * (double)v0[i];
			s1 += q * (double)v1[i];
			s2 += q * (double)v2[i];
			s3 += q * (double)v3[i];
			s4 += q * (double)v4[i];
			s5 += q * (double)v5[i];
			s6 += q * (double)v6[i];
			s7 += q * (double)v7[i];
		}
		out[0] = s0; out[1] = s1; out[2] = s2; out[3] = s3;
		out[4] = s4; out[5] = s5; out[6] = s6; out[7] = s7;
		return;
	}

	const float *vectors[8] = {v0, v1, v2, v3, v4, v5, v6, v7};
    for (size_t vector_index = 0; vector_index < count; ++vector_index) {
        double sum = 0.0;
        const float *vector = vectors[vector_index];
        for (size_t i = 0; i < n; ++i) {
            sum += (double)query[i] * (double)vector[i];
        }
        out[vector_index] = sum;
    }
}
*/
import "C"

import "unsafe"

func dotFloat32Accelerated(a, b []float32) float64 {
	return float64(C.hnsw_dot_f32(
		(*C.float)(unsafe.Pointer(&a[0])),
		(*C.float)(unsafe.Pointer(&b[0])),
		C.size_t(len(a)),
	))
}

func dotFloat32Batch(query []float32, vectors [][]float32, output []float64) {
	for start := 0; start < len(vectors); start += 8 {
		count := min(8, len(vectors)-start)
		pointers := [8]*C.float{}
		for i := range pointers {
			// Unused arguments point at the query. C only reads the first count
			// pointers, and using a valid pointer avoids passing nil to generated
			// cgo wrappers on platforms that instrument pointer arguments.
			pointers[i] = (*C.float)(unsafe.Pointer(&query[0]))
		}
		for i := 0; i < count; i++ {
			pointers[i] = (*C.float)(unsafe.Pointer(&vectors[start+i][0]))
		}
		C.hnsw_dot_f32_x8(
			(*C.float)(unsafe.Pointer(&query[0])),
			C.size_t(len(query)),
			C.size_t(count),
			pointers[0], pointers[1], pointers[2], pointers[3],
			pointers[4], pointers[5], pointers[6], pointers[7],
			(*C.double)(unsafe.Pointer(&output[start])),
		)
	}
}
