package lowhttp

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"sync"
	"testing"
)

type gzipDecodeFailingReader struct {
	read bool
}

func (r *gzipDecodeFailingReader) Read(p []byte) (int, error) {
	if !r.read {
		r.read = true
		return copy(p, "partial"), nil
	}
	return 0, errors.New("injected read failure")
}

func gzipDecodeAllocationFixture(b testing.TB, raw []byte) []byte {
	b.Helper()
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	if _, err := w.Write(raw); err != nil {
		b.Fatal(err)
	}
	if err := w.Close(); err != nil {
		b.Fatal(err)
	}
	return compressed.Bytes()
}

func legacyReadAllLimited(r io.Reader, max int) ([]byte, bool) {
	raw, err := io.ReadAll(io.LimitReader(r, int64(max)+1))
	if err != nil || len(raw) > max {
		return nil, false
	}
	return raw, true
}

func legacyGzipDecodeForBenchmark(compressed []byte, max int) ([]byte, bool) {
	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return compressed, false
	}
	defer r.Close()
	return legacyReadAllLimited(r, max)
}

func gzipSizeHintFreshReaderForBenchmark(compressed []byte, max int) ([]byte, bool) {
	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return compressed, false
	}
	defer r.Close()
	if out, ok := _readAllLimitedWithHint(r, max, _gzipDecodedSizeHint(compressed)); ok {
		return out, true
	}
	return compressed, false
}

func TestGzipDecodedSizeHintAndLimit(t *testing.T) {
	raw := bytes.Repeat([]byte("validated-gzip-body-"), 16*1024)
	compressed := gzipDecodeAllocationFixture(t, raw)
	if got := _gzipDecodedSizeHint(compressed); got != len(raw) {
		t.Fatalf("_gzipDecodedSizeHint() = %d, want %d", got, len(raw))
	}

	decoded, ok := _decodeBody(_contentAlgoGzip, compressed, len(raw))
	if !ok || !bytes.Equal(decoded, raw) {
		t.Fatal("gzip body did not decode at the exact limit")
	}
	decoded, ok = _decodeBody(_contentAlgoGzip, compressed, len(raw)-1)
	if ok || !bytes.Equal(decoded, compressed) {
		t.Fatal("gzip body exceeding the limit did not retain the compressed input")
	}
}

func TestGzipDecodedSizeHintIsSpeculativelyBounded(t *testing.T) {
	trailer := []byte{0xff, 0xff, 0xff, 0xff}
	if got := _gzipDecodedSizeHint(trailer); got != _decodedBodyInitialCapacityLimit {
		t.Fatalf("untrusted size hint = %d, want bounded %d", got, _decodedBodyInitialCapacityLimit)
	}
}

func TestGzipSizeHintKeepsMultistreamSemantics(t *testing.T) {
	first := bytes.Repeat([]byte("first-"), 32*1024)
	second := bytes.Repeat([]byte("second-"), 4*1024)
	compressed := append(gzipDecodeAllocationFixture(t, first), gzipDecodeAllocationFixture(t, second)...)
	want := append(bytes.Clone(first), second...)

	decoded, ok := _decodeBody(_contentAlgoGzip, compressed, len(want))
	if !ok || !bytes.Equal(decoded, want) {
		t.Fatal("concatenated gzip members changed under a last-member size hint")
	}
}

func TestGzipSizeHintKeepsInvalidStreamFallback(t *testing.T) {
	compressed := gzipDecodeAllocationFixture(t, bytes.Repeat([]byte("payload"), 1024))
	// Corrupt the checksum while preserving the ISIZE trailer used by the hint.
	compressed[len(compressed)-8] ^= 0xff

	decoded, ok := _decodeBody(_contentAlgoGzip, compressed, _autoUnzipMaxDecodedBodyBytes)
	if ok || !bytes.Equal(decoded, compressed) {
		t.Fatal("invalid gzip stream did not retain the compressed input")
	}
}

func TestGzipReaderPoolRecoversAfterDecodeErrors(t *testing.T) {
	raw := bytes.Repeat([]byte("valid-after-errors-"), 16*1024)
	compressed := gzipDecodeAllocationFixture(t, raw)

	invalidHeader := bytes.Clone(compressed)
	invalidHeader[0] ^= 0xff
	if decoded, ok := _decodeBody(_contentAlgoGzip, invalidHeader, len(raw)); ok || !bytes.Equal(decoded, invalidHeader) {
		t.Fatal("invalid gzip header did not retain its input")
	}

	invalidChecksum := bytes.Clone(compressed)
	invalidChecksum[len(invalidChecksum)-8] ^= 0xff
	if decoded, ok := _decodeBody(_contentAlgoGzip, invalidChecksum, len(raw)); ok || !bytes.Equal(decoded, invalidChecksum) {
		t.Fatal("invalid gzip checksum did not retain its input")
	}

	decoded, ok := _decodeBody(_contentAlgoGzip, compressed, len(raw))
	if !ok || !bytes.Equal(decoded, raw) {
		t.Fatal("pooled gzip reader did not recover after decode errors")
	}
}

func TestGzipReaderPoolConcurrentDecode(t *testing.T) {
	raw := bytes.Repeat([]byte("parallel-gzip-body-"), 8*1024)
	compressed := gzipDecodeAllocationFixture(t, raw)
	const workers = 32
	const iterations = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				decoded, ok := _decodeBody(_contentAlgoGzip, compressed, len(raw))
				if !ok || !bytes.Equal(decoded, raw) {
					errs <- errors.New("concurrent pooled gzip decode changed output")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestReadAllLimitedWithHintKeepsReaderErrorSemantics(t *testing.T) {
	decoded, ok := _readAllLimitedWithHint(&gzipDecodeFailingReader{}, 1024, 512)
	if ok || decoded != nil {
		t.Fatal("reader error must discard partial output")
	}
}

var (
	benchmarkGzipDecoded []byte
	benchmarkGzipOK      bool
)

func BenchmarkGzipDecodeSizeHint256K(b *testing.B) {
	raw := bytes.Repeat([]byte(`{"status":"ok","payload":"aaaaaaaaaaaaaaaa"}`), 262144/44+1)
	raw = raw[:262144]
	compressed := gzipDecodeAllocationFixture(b, raw)

	b.Run("legacy-read-all", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(raw)))
		for i := 0; i < b.N; i++ {
			benchmarkGzipDecoded, benchmarkGzipOK = legacyGzipDecodeForBenchmark(compressed, _autoUnzipMaxDecodedBodyBytes)
		}
	})

	b.Run("gzip-size-hint", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(raw)))
		for i := 0; i < b.N; i++ {
			benchmarkGzipDecoded, benchmarkGzipOK = _decodeBody(_contentAlgoGzip, compressed, _autoUnzipMaxDecodedBodyBytes)
		}
	})
}

func BenchmarkGzipReaderPool256K(b *testing.B) {
	raw := bytes.Repeat([]byte(`{"status":"ok","payload":"aaaaaaaaaaaaaaaa"}`), 262144/44+1)
	raw = raw[:262144]
	compressed := gzipDecodeAllocationFixture(b, raw)

	b.Run("fresh-reader", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(raw)))
		for i := 0; i < b.N; i++ {
			benchmarkGzipDecoded, benchmarkGzipOK = gzipSizeHintFreshReaderForBenchmark(compressed, _autoUnzipMaxDecodedBodyBytes)
		}
	})

	b.Run("pooled-reader", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(raw)))
		for i := 0; i < b.N; i++ {
			benchmarkGzipDecoded, benchmarkGzipOK = _decodeBody(_contentAlgoGzip, compressed, _autoUnzipMaxDecodedBodyBytes)
		}
	})
}
