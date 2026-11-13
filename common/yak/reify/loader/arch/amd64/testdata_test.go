package amd64

import (
	"context"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"strconv"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/asm2lowir"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

//go:embed testdata
var testdata embed.FS

type fixture struct {
	Name    string       `json:"name"`
	Address string       `json:"address"`
	Symbols []fixtureSym `json:"symbols"`
	Code    []string     `json:"code"`
	LowIR   []string     `json:"lowir"`
}

type fixtureSym struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

func TestFixtures(t *testing.T) {
	err := filesys.Recursive("testdata", filesys.WithEmbedFS(testdata), filesys.WithFileStat(func(path string, info fs.FileInfo) error {
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".json") {
			return nil
		}
		data, err := testdata.ReadFile(path)
		if err != nil {
			return err
		}
		t.Run(strings.TrimSuffix(info.Name(), ".json"), func(t *testing.T) {
			runFixtures(t, data)
		})
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
}

func runFixtures(t *testing.T, data []byte) {
	t.Helper()
	var cases []fixture
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			address := parseAddress(t, tc.Address)
			code := decodeFixtureCode(t, tc.Code)
			reader, err := core.NewByteReader(tc.Name, strings.NewReader(string(code)), int64(len(code)))
			if err != nil {
				t.Fatal(err)
			}
			image := &core.Image{Sections: []core.Section{{
				Name:           ".text",
				FileOffset:     0,
				FileSize:       uint64(len(code)),
				VirtualAddress: address,
				MemorySize:     uint64(len(code)),
			}}}
			for _, item := range tc.Symbols {
				image.Symbols = append(image.Symbols, sym.Symbol{
					Name:    item.Name,
					Address: parseAddress(t, item.Address),
					Kind:    sym.SymbolFunction,
				})
			}
			artifact := &core.Artifact{
				Origin: core.Origin{Name: tc.Name, Size: int64(len(code))},
				Kind:   core.KindNative,
				Format: core.FormatELF,
				Target: core.Target{OS: core.OSLinux, Architecture: core.ArchAMD64, Bits: 64, ByteOrder: core.LittleEndian},
				Image:  image,
				Reader: reader,
				Parser: Parser{},
				Hints:  &core.BoundaryHints{ExecSections: []core.Region{{Address: address, Size: uint64(len(code))}}},
			}
			options := []asm2lowir.Option{}
			if len(tc.Symbols) > 0 {
				options = append(options, asm2lowir.WithAll())
			} else {
				options = append(options,
					asm2lowir.WithRegion(core.Region{Address: address, Size: uint64(len(code))}),
					asm2lowir.WithExact(),
				)
			}
			result, err := asm2lowir.Parse(context.Background(), artifact, options...)
			if err != nil {
				t.Fatal(err)
			}
			got := strings.TrimRight(lowirtext.Format(result.IR), "\n")
			want := strings.Join(tc.LowIR, "\n")
			if got != want {
				t.Fatalf("LowIR mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
			}
		})
	}
}

func decodeFixtureCode(t *testing.T, lines []string) []byte {
	t.Helper()
	var code []byte
	for _, line := range lines {
		bytesPart, comment, hasComment := strings.Cut(line, "//")
		if !hasComment || strings.TrimSpace(comment) == "" {
			t.Fatalf("line without an assembly comment: %q", line)
		}
		decoded, err := hex.DecodeString(strings.Join(strings.Fields(bytesPart), ""))
		if err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		code = append(code, decoded...)
	}
	if len(code) == 0 {
		t.Fatal("no bytes")
	}
	return code
}

func parseAddress(t *testing.T, text string) uint64 {
	t.Helper()
	if text == "" {
		return 0
	}
	value, err := strconv.ParseUint(text, 0, 64)
	if err != nil {
		t.Fatalf("bad address %q: %v", text, err)
	}
	return value
}
