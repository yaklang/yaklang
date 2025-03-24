package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Panic(t *testing.T) {
	t.Run("anonymous type panic in range next", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
	
			type A []int
	
			func (p *A) AcceptToken() (tok token.Token, ok bool) {
				for _, t := range p {
					if tok.Type == t {
						return tok, true
					}
				}
				return tok, false
			}
			`, []string{}, t)
	})

	t.Run("anonymous type panic in ExternMethodBuilder", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

			func (p *int) AcceptToken() (tok token.Token, ok bool) {
				tok = p.ReadToken()
				return tok, false
			}
			`, []string{}, t)
	})

	t.Run("interface type overwrite panic in build", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("src/main/go/go.mod", `
		module github.com/ollama/ollama/
	
		go 1.20
		`)
		vf.AddFile("src/main/go/convert/convert2.go", `
	package convert
	
	import (
		"cmp"
		"encoding/json"
		"io/fs"
		"path/filepath"
		"slices"
		"strings"
	
		"github.com/ollama/ollama/fs/ggml"
	)
	
	type AdapterConverter interface {
		// KV maps parameters to LLM key-values
		KV(ggml.KV) ggml.KV
		// Tensors maps input tensors to LLM tensors. Adapter specific modifications can be done here.
		Tensors([]Tensor) []ggml.Tensor
		// Replacements returns a list of string pairs to replace in tensor names.
		// See [strings.Replacer](https://pkg.go.dev/strings#Replacer) for details
		Replacements() []string
	
		writeFile(io.WriteSeeker, ggml.KV, []ggml.Tensor) error
	}
			`)
		vf.AddFile("src/main/go/convert/convert.go", `
	package convert
	
	import (
		"errors"
		"io"
		"io/fs"
		"strings"
		"test/ggml"
	)
	
	type Tensor interface {
		Name() string
		Shape() []uint64
		Kind() uint32
		SetRepacker(repacker)
		WriteTo(io.Writer) (int64, error)
	}
		`)

		test.CheckSyntaxFlowWithFS(t, vf,
			`Tensor as $a`,
			map[string][]string{
				"a": {""},
			}, true, ssaapi.WithLanguage(ssaapi.GO))
	})

}
