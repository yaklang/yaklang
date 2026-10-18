package minirehs

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func syntheticImagePatterns() []Pattern {
	pats := make([]Pattern, 256)
	for i := range pats {
		pats[i] = Pattern{ID: PatternID(i + 1), Expr: fmt.Sprintf(`(?i)header-%03d:\s*[a-z0-9]{4,16}(?:;[a-z]{2,8})?`, i)}
	}
	return pats
}

// Read the repository's ordered GetRequest rules, including the bundled HTTP
// softmatch appended by fp. This does not instantiate the fp scanner.
// The match-block delimiter and \0 normalization mirror fp's nmap parser.
func getRequestImagePatterns(tb testing.TB) []Pattern {
	tb.Helper()
	f, err := os.Open(filepath.Join("..", "..", "embed", "data", "nfp.gz"))
	if err != nil {
		tb.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		tb.Fatal(err)
	}
	defer gz.Close()
	extra, err := os.ReadFile(filepath.Join("..", "..", "embed", "data", "user-fp-rules", "tcp-http-get-payload.txt"))
	if err != nil {
		tb.Fatal(err)
	}
	sc := bufio.NewScanner(io.MultiReader(gz, strings.NewReader("\n"), bytes.NewReader(extra)))
	sc.Buffer(make([]byte, 4096), 2<<20)
	active := false
	var out []Pattern
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "Probe ") {
			active = strings.HasPrefix(line, "Probe TCP GetRequest ")
			continue
		}
		if !active || !(strings.HasPrefix(line, "match ") || strings.HasPrefix(line, "softmatch ")) {
			continue
		}
		parts := strings.SplitN(line, " ", 3)
		if len(parts) != 3 || len(parts[2]) < 3 || parts[2][0] != 'm' {
			tb.Fatalf("invalid GetRequest rule: %s", line)
		}
		block := parts[2]
		end := strings.IndexByte(block[2:], block[1]) + 2
		if end < 2 {
			tb.Fatalf("unterminated GetRequest rule: %s", line)
		}
		expr := strings.ReplaceAll(block[2:end], `\0`, `\x00`)
		flags := Flag(0)
		opt := strings.SplitN(block[end+1:], " ", 2)[0]
		if strings.Contains(opt, "i") {
			flags |= FlagCaseless
		}
		if strings.Contains(opt, "s") {
			flags |= FlagDotAll
		}
		out = append(out, Pattern{ID: PatternID(len(out) + 1), Expr: expr, Flags: flags})
	}
	if err = sc.Err(); err != nil {
		tb.Fatal(err)
	}
	if len(out) < 200 {
		tb.Fatalf("GetRequest dataset unexpectedly small: %d", len(out))
	}
	return out
}

func BenchmarkCompilationImage(b *testing.B) {
	for _, name := range []string{"Synthetic256", "MITM", "GetRequest"} {
		b.Run(name, func(b *testing.B) {
			var pats []Pattern
			switch name {
			case "Synthetic256":
				pats = syntheticImagePatterns()
			case "MITM":
				pats, _ = compilableMITMPatterns(b)
			case "GetRequest":
				pats = getRequestImagePatterns(b)
			}
			opts := []Option{WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{})}
			compiled, err := Compile(pats, opts...)
			if err != nil {
				b.Fatal(err)
			}
			defer compiled.Close()
			data, err := compiled.MarshalImage()
			if err != nil {
				b.Fatal(err)
			}
			loaded, err := LoadImage(data)
			if err != nil {
				b.Fatal(err)
			}
			defer loaded.Close()
			dense, err := compiled.(*database).marshalUncompressedImage()
			if err != nil {
				b.Fatal(err)
			}
			packed, err := compiled.(*database).marshalCompilationImage(true)
			if err != nil {
				b.Fatal(err)
			}
			b.Logf("patterns=%d image=%d bytes (%.1f bytes/pattern)", len(pats), len(data), float64(len(data))/float64(len(pats)))
			path := filepath.Join(b.TempDir(), "cache.rehs")
			if err = writeImageCache(path, data); err != nil {
				b.Fatal(err)
			}
			record := bytes.Repeat([]byte("HTTP/1.1 200 OK\r\nServer: nginx/1.24.0\r\nContent-Type: text/html\r\n\r\n<html> token=abc123 header-042: abcd;ef </html>\n"), 8)
			for _, op := range []string{"Compile", "LoadImage", "LoadAndFirstScan", "MarshalImage", "CacheHit", "CacheMiss", "ReadImageFile", "WriteImageFile", "MarshalPackedImage", "LoadPackedImage", "LoadDenseImage"} {
				b.Run(op, func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					b.ReportMetric(float64(len(data)), "image-B")
					b.ReportMetric(float64(len(pats)), "patterns")
					for i := 0; i < b.N; i++ {
						var db Database
						switch op {
						case "Compile":
							db, err = Compile(pats, opts...)
						case "LoadImage":
							db, err = LoadImage(data)
						case "LoadDenseImage":
							db, err = LoadImage(dense)
						case "LoadPackedImage":
							db, err = LoadImage(packed)
						case "MarshalPackedImage":
							_, err = compiled.(*database).marshalCompilationImage(true)
						case "LoadAndFirstScan":
							db, err = LoadImage(data)
							if err == nil {
								err = db.Scan(record, nil, nil)
							}
						case "MarshalImage":
							_, err = compiled.MarshalImage()
						case "CacheHit":
							db, err = LoadOrCompile(path, pats, opts...)
						case "CacheMiss":
							err = os.Remove(path)
							if err == nil {
								db, err = LoadOrCompile(path, pats, opts...)
							}
						case "ReadImageFile":
							_, err = readImageFile(path)
						case "WriteImageFile":
							err = writeImageCache(path, data)
						}
						if err != nil {
							b.Fatal(err)
						}
						if db != nil {
							_ = db.Close()
						}
					}
				})
			}
			want, e := imageMatches(compiled, record)
			got, e2 := imageMatches(loaded, record)
			if e != nil || e2 != nil || !reflect.DeepEqual(want, got) {
				b.Fatal("benchmark scan results differ")
			}
			for _, pair := range []struct {
				name string
				db   Database
			}{{"ScanCompiled", compiled}, {"ScanLoaded", loaded}} {
				b.Run(pair.name, func(b *testing.B) {
					sc, err := pair.db.NewScratch()
					if err != nil {
						b.Fatal(err)
					}
					defer sc.Close()
					_ = pair.db.Scan(record, sc, nil)
					b.SetBytes(int64(len(record)))
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if err = pair.db.Scan(record, sc, nil); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
