package test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// The standard Go type checker is the oracle for legal and ambiguous selectors.
func TestGoEmbeddedFieldSelectors(t *testing.T) {
	cases := []struct {
		name    string
		code    string
		want    []string
		invalid bool
	}{
		{
			name: "single_value_read_control",
			code: `package main
type Model struct { ID int }
type History struct { Model }
func main() { h := History{Model: Model{ID: 41}}; println(h.ID); println(h.Model.ID) }`,
			want: []string{"41", "41"},
		},
		{
			name: "nested_value_read",
			code: `package main
type Model struct { ID int }
type Flow struct { Model }
type History struct { Flow }
func main() { h := History{Flow: Flow{Model: Model{ID: 41}}}; promoted := h.ID; explicit := h.Flow.Model.ID; println(promoted); println(explicit) }`,
			want: []string{"41", "41"},
		},
		{
			name: "nested_value_write",
			code: `package main
type Model struct { ID int }
type Flow struct { Model }
type History struct { Flow }
func main() { h := History{Flow: Flow{Model: Model{ID: 41}}}; h.ID = 42; println(h.ID); println(h.Flow.Model.ID) }`,
			want: []string{"42", "42"},
		},
		{
			name: "nested_pointer_read",
			code: `package main
type Model struct { ID int }
type Flow struct { *Model }
type History struct { *Flow }
func main() { h := History{Flow: &Flow{Model: &Model{ID: 41}}}; promoted := h.ID; explicit := h.Flow.Model.ID; println(promoted); println(explicit) }`,
			want: []string{"41", "41"},
		},
		{
			name: "slice_pointer_nested_read",
			code: `package main
type Model struct { ID int }
type Flow struct { *Model }
type History struct { *Flow; DatabaseID int64 }
type Page struct { Items []*History }
func main() { page := Page{Items: []*History{&History{Flow: &Flow{Model: &Model{ID: 41}}, DatabaseID: 7}}}; println(page.Items[0].ID); println(page.Items[0].Flow.Model.ID); println(page.Items[0].DatabaseID) }`,
			want: []string{"41", "41", "7"},
		},
		{
			name: "multiple_value_reads_control",
			code: `package main
type Model struct { ID int }
type Metadata struct { Label string }
type History struct { Model; Metadata }
func main() { h := History{Model: Model{ID: 41}, Metadata: Metadata{Label: "tag"}}; println(h.ID); println(h.Label) }`,
			want: []string{"41", `"tag"`},
		},
		{
			name: "multiple_value_writes",
			code: `package main
type Model struct { ID int }
type Metadata struct { Label string }
type History struct { Model; Metadata }
func main() { h := History{Model: Model{ID: 41}, Metadata: Metadata{Label: "tag"}}; h.ID = 42; h.Label = "ok"; println(h.Model.ID); println(h.Metadata.Label) }`,
			want: []string{"42", `"ok"`},
		},
		{
			name: "direct_field_shadows_control",
			code: `package main
type Model struct { ID int }
type Flow struct { Model }
type History struct { Flow; ID string }
func main() { h := History{Flow: Flow{Model: Model{ID: 41}}, ID: "direct"}; println(h.ID); println(h.Flow.Model.ID) }`,
			want: []string{`"direct"`, "41"},
		},
		{
			name: "shallower_field_wins",
			code: `package main
type Model struct { ID int }
type Flow struct { Model }
type Other struct { ID string }
type History struct { Flow; Other }
func main() { h := History{Flow: Flow{Model: Model{ID: 41}}, Other: Other{ID: "shallow"}}; println(h.ID); println(h.Other.ID) }`,
			want: []string{`"shallow"`, `"shallow"`},
		},
		{
			name: "single_pointer_write_alias",
			code: `package main
type Model struct { ID int }
type History struct { *Model }
func main() { m := Model{ID: 41}; h := History{Model: &m}; h.ID = 42; println(m.ID); println(h.Model.ID) }`,
			want: []string{"42", "42"},
		},
		{
			name: "ambiguous_same_depth_read",
			code: `package main
type Left struct { ID int }
type Right struct { ID int }
type History struct { Left; Right }
func main() { h := History{Left: Left{ID: 41}, Right: Right{ID: 42}}; println(h.ID) }`,
			invalid: true,
		},
		{
			name: "ambiguous_same_depth_write",
			code: `package main
type Left struct { ID int }
type Right struct { ID int }
type History struct { Left; Right }
func main() { h := History{Left: Left{ID: 41}, Right: Right{ID: 42}}; h.ID = 43; println(h.Left.ID); println(h.Right.ID) }`,
			invalid: true,
			want:    []string{"41", "42"},
		},
		{
			name: "diamond_ambiguity",
			code: `package main
type Model struct { ID int }
type Left struct { Model }
type Right struct { Model }
type History struct { Left; Right }
func main() { h := History{Left: Left{Model: Model{ID: 41}}, Right: Right{Model: Model{ID: 42}}}; println(h.ID) }`,
			invalid: true,
		},
		{
			name: "named_field_no_promotion_control",
			code: `package main
type Model struct { ID int }
type History struct { Model Model }
func main() { h := History{Model: Model{ID: 41}}; println(h.ID) }`,
			invalid: true,
		},
		{
			name: "parameter_nested_type",
			code: `package main
type Model struct { ID int }
type Flow struct { Model }
type History struct { Flow }
func inspect(h History) { promoted := h.ID; explicit := h.Flow.Model.ID; println(promoted); println(explicit) }`,
		},
		{
			name: "nested_zero_value",
			code: `package main
type Model struct { ID int }
type Flow struct { Model }
type History struct { Flow }
func main() { h := History{}; println(h.ID); println(h.Flow.Model.ID); h.ID = 42; println(h.ID); println(h.Flow.Model.ID) }`,
			want: []string{"0", "0", "42", "42"},
		},
		{
			name: "nested_unkeyed_literals",
			code: `package main
type Model struct { ID int }
type Flow struct { Model }
type History struct { Flow }
func main() { h := History{Flow{Model{41}}}; h.ID++; println(h.ID); println(h.Flow.Model.ID) }`,
			want: []string{"42", "42"},
		},
		{
			name: "embedded_alias",
			code: `package main
type Model struct { ID int }
type Alias = Model
type History struct { Alias }
func main() { h := History{Alias: Alias{ID: 41}}; h.ID += 1; println(h.ID); println(h.Alias.ID) }`,
			want: []string{"42", "42"},
		},
		{
			name: "nested_pointer_alias_writes",
			code: `package main
type Model struct { ID int }
type Flow struct { *Model }
type History struct { *Flow }
func main() { m := Model{ID: 41}; f := Flow{Model: &m}; h := History{Flow: &f}; h.ID = 42; println(m.ID); println(f.ID); println(h.ID); h.Flow.Model.ID = 43; println(m.ID); println(h.ID); m.ID = 44; println(h.ID) }`,
			want: []string{"42", "42", "42", "43", "43", "44"},
		},
		{
			name: "pointer_to_outer_struct",
			code: `package main
type Model struct { ID int }
type Flow struct { Model }
type History struct { Flow }
func main() { h := &History{Flow: Flow{Model: Model{ID: 41}}}; h.ID = 42; println(h.ID); println(h.Flow.Model.ID) }`,
			want: []string{"42", "42"},
		},
		{
			name: "pointer_embedding_survives_value_copy",
			code: `package main
type Model struct { ID int }
type History struct { *Model }
type Envelope struct { History }
func main() { m := Model{ID: 41}; h := History{Model: &m}; e := Envelope{History: h}; e.ID = 42; println(m.ID); println(h.ID); println(e.ID) }`,
			want: []string{"42", "42", "42"},
		},
		{
			name: "value_embedding_is_copied",
			code: `package main
type Model struct { ID int }
type History struct { Model }
func main() { m := Model{ID: 41}; h := History{Model: m}; h.ID = 42; println(m.ID); println(h.ID); println(h.Model.ID) }`,
			want: []string{"41", "42", "42"},
		},
		{
			name: "multiple_value_writes_reversed",
			code: `package main
type Model struct { ID int }
type Metadata struct { Label string }
type History struct { Metadata; Model }
func main() { h := History{Model: Model{ID: 41}, Metadata: Metadata{Label: "tag"}}; h.Label = "ok"; h.ID = 42; println(h.Model.ID); println(h.Metadata.Label) }`,
			want: []string{"42", `"ok"`},
		},
		{
			name: "shallower_field_at_second_depth",
			code: `package main
type Model struct { ID int }
type Deep struct { Model }
type Deeper struct { Deep }
type Other struct { ID string }
type Shallow struct { Other }
type History struct { Deeper; Shallow }
func main() { h := History{Deeper: Deeper{Deep: Deep{Model: Model{ID: 41}}}, Shallow: Shallow{Other: Other{ID: "shallow"}}}; h.ID = "updated"; println(h.ID); println(h.Shallow.Other.ID); println(h.Deeper.Deep.Model.ID) }`,
			want: []string{`"updated"`, `"updated"`, "41"},
		},
		{
			name: "direct_field_shadows_diamond",
			code: `package main
type Model struct { ID int }
type Left struct { Model }
type Right struct { Model }
type History struct { Left; Right; ID string }
func main() { h := History{Left: Left{Model: Model{ID: 41}}, Right: Right{Model: Model{ID: 42}}, ID: "direct"}; h.ID = "updated"; println(h.ID); println(h.Left.Model.ID); println(h.Right.Model.ID) }`,
			want: []string{`"updated"`, "41", "42"},
		},
		{
			name: "named_field_write_no_promotion",
			code: `package main
type Model struct { ID int }
type History struct { Model Model }
func main() { h := History{Model: Model{ID: 41}}; h.ID = 42; println(h.Model.ID) }`,
			invalid: true,
			want:    []string{"41"},
		},
		{
			name: "parameter_pointer_nested_type",
			code: `package main
type Model struct { ID int }
type Flow struct { *Model }
type History struct { *Flow }
func inspect(h History) { promoted := h.ID; explicit := h.Flow.Model.ID; println(promoted); println(explicit) }`,
		},
		{
			name: "slice_pointer_nested_write",
			code: `package main
type Model struct { ID int }
type Flow struct { *Model }
type History struct { *Flow }
type Page struct { Items []*History }
func main() { m := Model{ID: 41}; h := History{Flow: &Flow{Model: &m}}; page := Page{Items: []*History{&h}}; page.Items[0].ID = 42; println(m.ID); println(h.ID); println(page.Items[0].Flow.Model.ID) }`,
			want: []string{"42", "42", "42"},
		},
		{
			name: "nested_value_embedding_is_copied",
			code: `package main
type Model struct { ID int }
type History struct { Model }
type Envelope struct { History }
func main() { h := History{Model: Model{ID: 41}}; e := Envelope{History: h}; e.ID = 42; println(h.ID); println(e.ID); println(e.History.Model.ID) }`,
			want: []string{"41", "42", "42"},
		},
		{
			name: "embedded_basic_alias",
			code: `package main
type Name string
type History struct { Name }
func main() { h := History{Name: "tag"}; println(h.Name) }`,
			want: []string{`"tag"`},
		},
		{
			name: "embedded_basic_type",
			code: `package main
type History struct { int }
func main() { h := History{int: 41}; println(h.int) }`,
			want: []string{"41"},
		},
		{
			name: "array_pointer_nested_write",
			code: `package main
type Model struct { ID int }
type History struct { *Model }
type Page struct { Items [1]*History }
func main() { m := Model{ID: 41}; h := History{Model: &m}; page := Page{Items: [1]*History{&h}}; page.Items[0].ID = 42; println(m.ID); println(h.ID); println(page.Items[0].Model.ID) }`,
			want: []string{"42", "42", "42"},
		},
		{
			name: "ellipsis_array_pointer_nested_write",
			code: `package main
type Model struct { ID int }
type History struct { *Model }
func main() { m := Model{ID: 41}; h := History{Model: &m}; items := [...]*History{&h}; items[0].ID = 42; println(m.ID); println(h.ID); println(items[0].Model.ID) }`,
			want: []string{"42", "42", "42"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "main.go", tc.code, 0)
			require.NoError(t, err)
			config := types.Config{}
			_, goErr := config.Check("main", fset, []*ast.File{file}, nil)
			if tc.invalid {
				require.Error(t, goErr, "invalid fixture must be rejected by Go")
			} else {
				require.NoError(t, goErr, "valid fixture must be accepted by Go")
			}
			t.Logf("Go type checker: %v", goErr)
			prog, err := ssaapi.Parse(tc.code, ssaapi.WithLanguage(ssaconfig.GO))
			require.NoError(t, err)
			var diagnostics []string
			for _, diagnostic := range prog.GetErrors() {
				if diagnostic.Kind == ssa.Error {
					diagnostics = append(diagnostics, diagnostic.Message)
				}
			}
			var got []string
			for _, call := range prog.Ref("println").GetUsers() {
				if operand := call.GetOperand(1); operand != nil {
					got = append(got, operand.String())
				}
			}
			t.Logf("SSA errors: %v; println values: %v", diagnostics, got)
			if tc.want != nil {
				assert.Equal(t, tc.want, got)
			}
			if tc.invalid {
				require.NotEmpty(t, diagnostics, "go2ssa must reject the selector rejected by Go")
				if strings.Contains(goErr.Error(), "ambiguous selector") {
					assert.Contains(t, strings.ToLower(strings.Join(diagnostics, "\n")), "ambiguous", "ambiguity requires a semantic diagnostic")
				}
				for _, diagnostic := range diagnostics {
					assert.NotContains(t, diagnostic, "[BUG]", "a frontend internal error is not a semantic diagnostic")
				}
				return
			}
			for _, name := range []string{"promoted", "explicit"} {
				if strings.Contains(tc.code, name+" :=") {
					require.Len(t, prog.Ref(name), 1, name)
				}
				for _, value := range prog.Ref(name) {
					t.Logf("%s: value=%s type=%s kind=%v", name, value.String(), value.GetType().String(), value.GetTypeKind())
					assert.Equal(t, ssa.NumberTypeKind, value.GetTypeKind(), name+" must have numeric field type")
				}
			}
			assert.Empty(t, diagnostics, "valid Go must not produce SSA errors")
		})
	}
}

func TestGoEmbeddedFieldsAcrossPackages(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("go.mod", "module example.com/embedded\n\ngo 1.22\n")
	fs.AddFile("models/model.go", `package models
type Model struct { ID int }
type Flow struct { *Model }
`)
	fs.AddFile("main.go", `package main
import "example.com/embedded/models"
type History struct { *models.Flow }
func main() {
	m := models.Model{ID: 41}
	f := models.Flow{Model: &m}
	h := History{Flow: &f}
	promoted := h.ID
	explicit := h.Flow.Model.ID
	println(promoted)
	println(explicit)
	h.ID = 42
	println(m.ID)
	println(h.Flow.Model.ID)
}
`)
	// Exercise all source orders, compilation with persistence, and database
	// reload. The imported embedding's field name is Flow, not models.Flow.
	ssatest.CheckWithFS(fs, t, func(programs ssaapi.Programs) error {
		require.Len(t, programs, 1)
		prog := programs[0]
		for _, diagnostic := range prog.GetErrors() {
			assert.NotEqual(t, ssa.Error, diagnostic.Kind, diagnostic.Message)
		}
		var got []string
		for _, call := range prog.Ref("println").GetUsers() {
			if operand := call.GetOperand(1); operand != nil {
				got = append(got, operand.String())
			}
		}
		require.Equal(t, []string{"41", "41", "42", "42"}, got)
		for _, name := range []string{"promoted", "explicit"} {
			values := prog.Ref(name)
			require.Len(t, values, 1, name)
			require.Equal(t, ssa.NumberTypeKind, values[0].GetTypeKind(), name)
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))
}
