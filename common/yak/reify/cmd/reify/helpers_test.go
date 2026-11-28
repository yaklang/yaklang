package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

func TestWriteInspectionJSONMatchesWriteJSONByteForByte(t *testing.T) {
	entry := uint64(0x401000)
	fullReport := inspectionReport{
		Scope: "metadata-only",
		Artifact: &core.Artifact{
			Origin: core.Origin{Name: "app.exe", Size: 4096},
			Kind:   core.KindNative,
			Format: core.FormatPE,
			Target: core.Target{
				OS:           core.OSWindows,
				Architecture: core.ArchAMD64,
				Bits:         64,
				ByteOrder:    core.LittleEndian,
				ABI:          "pe-subsystem-3",
				Machine:      "0x8664",
			},
			Image: &core.Image{
				ObjectType: "PE32+",
				Base:       0x400000,
				Entry:      &entry,
				Sections: []core.Section{
					{Name: ".text", FileOffset: 0x400, FileSize: 0x200, VirtualAddress: 0x1000, MemorySize: 0x200, Flags: 0x60000020},
					{Name: ".data", FileOffset: 0x600, FileSize: 0x100, VirtualAddress: 0x2000, MemorySize: 0x100, Writable: true},
				},
				Symbols: []sym.Symbol{
					{
						Name:    "main.Box[int].Get",
						Mangled: "main.Box[go.shape.int].Get",
						Address: 0x401000,
						Size:    64,
						Kind:    sym.SymbolFunction,
						Source:  sym.SourcePCLNTab,
						Identity: &sym.Identity{
							Package:  "main",
							Receiver: "Box[int]",
							Function: "Get",
							Family:   "main.Box[T].Get",
							TypeArgs: []string{"int"},
							Generic:  true,
						},
					},
					{Name: "VirtualAlloc", Kind: sym.SymbolFunction, Imported: true, Library: "KERNEL32.dll"},
				},
				Relocations: []core.Relocation{
					{Address: 0x402000, Value: 0x401000, Symbol: "main.Box[int].Get"},
				},
			},
			Diagnostics: []core.Diagnostic{{Code: "note", Message: "test & <xml>"}},
			Language: &core.LanguageReport{
				Language:   core.LanguageGo,
				Confidence: core.ConfidenceCertain,
				Runtime:    "go1.22.12",
				Evidence:   []core.LanguageEvidence{{Source: "pclntab", Detail: "2 functions"}},
				ABI: &core.ABI{
					Registers:    core.RegisterModel{Naming: core.RegisterNamingX86, Width: 64, Arguments: []string{"rax", "rbx"}, Return: "rax"},
					NoCalleeSave: true,
				},
				Strings: &core.Strings{Layout: core.StringLayoutPointerLength, Encoding: core.StringEncodingUTF8},
				Notes:   []string{"sample note"},
			},
		},
		Parser: &core.ParserInfo{Name: "x86-64", Kind: core.ArchAMD64, Available: true},
	}

	nilSlicesReport := inspectionReport{
		Scope: "metadata-only",
		Artifact: &core.Artifact{
			Origin: core.Origin{Name: "empty.bin", Size: 64},
			Kind:   core.KindNative,
			Format: core.FormatELF,
			Target: core.Target{OS: core.OSLinux, Architecture: core.ArchAMD64, Bits: 64, ByteOrder: core.LittleEndian},
			Image:  &core.Image{ObjectType: "ET_EXEC", Sections: nil},
		},
	}

	emptySlicesReport := inspectionReport{
		Scope: "metadata-only",
		Artifact: &core.Artifact{
			Origin: core.Origin{Name: "empty2.bin", Size: 64},
			Kind:   core.KindNative,
			Format: core.FormatELF,
			Target: core.Target{OS: core.OSLinux, Architecture: core.ArchAMD64, Bits: 64, ByteOrder: core.LittleEndian},
			Image:  &core.Image{ObjectType: "ET_EXEC", Sections: []core.Section{}},
		},
	}

	for name, r := range map[string]inspectionReport{
		"full":         fullReport,
		"nil_slices":   nilSlicesReport,
		"empty_slices": emptySlicesReport,
	} {
		t.Run(name, func(t *testing.T) {
			var want, got bytes.Buffer
			require.NoError(t, writeJSON(&want, r))
			require.NoError(t, writeInspectionJSON(&got, r))
			require.Equal(t, want.String(), got.String())
		})
	}
}
