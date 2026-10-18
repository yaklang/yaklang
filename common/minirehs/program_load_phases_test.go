package minirehs

import (
	"bytes"
	"testing"
)

var programLoadBytes []byte
var programLoadTables *mvsDB

// Independent component calls expose costs, not an additive partition: GC,
// cache state and surrounding validation differ from complete LoadImage.
func BenchmarkProgramLoadPhases(b *testing.B) {
	for _, set := range []string{"GetRequest", "MITM"} {
		b.Run(set, func(b *testing.B) {
			pats := getRequestImagePatterns(b)
			if set == "MITM" {
				pats, _ = compilableMITMPatterns(b)
			}
			db, err := Compile(pats, WithBackend(BackendMVS), WithLogger(silentLogger{}))
			if err != nil {
				b.Fatal(err)
			}
			defer db.Close()
			m := db.(*database).imageMVS()
			image, err := db.MarshalImage()
			if err != nil {
				b.Fatal(err)
			}
			h, err := readImageHeader(image)
			if err != nil || !h.compressed || !h.packed {
				b.Fatal("expected actual compressed packed image", err)
			}
			packed, err := packImageBlob(m.blob, m.n)
			if err != nil {
				b.Fatal(err)
			}
			firstSubject := programRuntimeInputs()["HTTP912"]
			for _, phase := range []string{"EncodedClone", "Decompress", "UnpackBlob", "DecodeGoTables", "NativeOpenClose", "Load", "LoadFirstHTTP912"} {
				b.Run(phase, func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						switch phase {
						case "EncodedClone":
							programLoadBytes = bytes.Clone(image)
						case "Decompress":
							programLoadBytes, err = decompressCompilationImage(image, h.rawSize)
						case "UnpackBlob":
							programLoadBytes, err = unpackImageBlob(packed, m.n)
						case "DecodeGoTables":
							d := &mvsDB{n: m.n, nfas: make([]*mvsNFA, m.n), assertNecFactor: make([]necFactor, m.n)}
							err = decodeImageBlob(m.blob, d)
							programLoadTables = d
						case "NativeOpenClose":
							k := openMVSKernel(m.blob, m.n)
							if k == nil {
								b.Fatal("native open failed")
							}
							k.close()
						default:
							var d Database
							d, err = LoadImage(image)
							if err == nil && phase == "LoadFirstHTTP912" {
								var sc Scratch
								sc, err = d.NewScratch()
								if err == nil {
									err = d.Scan(firstSubject, sc, nil)
									sc.Close()
								}
							}
							if d != nil {
								d.Close()
							}
						}
						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
