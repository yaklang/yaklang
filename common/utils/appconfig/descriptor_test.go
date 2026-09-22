package appconfig

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestDescriptors(t *testing.T) {
	type config struct {
		Hidden  string
		Enabled bool     `app:"id:3,required:true,desc:enabled"`
		Token   string   `app:"id:1,name:token,default:old,verbose:API token,extra:secret"`
		Count   int      `app:"id:2"`
		Choice  []string `app:"id:4,type:list"`
	}
	got, err := ParseAppTagToOptions(&config{}, map[string]string{"token": "default:first"}, map[string]string{"token": "default:last,required:true"})
	if err != nil {
		t.Fatal(err)
	}
	want := []*FieldDescriptor{
		{Name: "token", DefaultValue: "last", Verbose: "API token", Extra: "secret", Type: "string", Required: true},
		{Name: "Count", Verbose: "Count", Type: "number"},
		{Name: "Enabled", Verbose: "Enabled", Type: "bool", Required: true, Desc: "enabled"},
		{Name: "Choice", Verbose: "Choice", Type: "list"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestInvalidDescriptors(t *testing.T) {
	for _, v := range []any{nil, 1, new(int), &struct {
		V string `app:"id:bad"`
	}{}, &struct {
		V string `app:"unknown:x"`
	}{}, &struct {
		V string `app:"type:invalid"`
	}{}, &struct {
		V []string `app:"name:v"`
	}{}} {
		if _, err := ParseAppTagToOptions(v); err == nil {
			t.Errorf("%T: expected error", v)
		}
	}
	// Parsing needs the type, not a populated value.
	var typedNil *struct {
		V string `app:"name:v"`
	}
	if got, err := ParseAppTagToOptions(typedNil); err != nil || len(got) != 1 {
		t.Fatalf("typed nil: %v %v", got, err)
	}
}

// A tag-only consumer must not acquire service, database, or third-party imports.
func TestStandardLibraryDependencyBoundary(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", "github.com/yaklang/yaklang/common/utils/appconfig")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	if got := strings.Fields(string(out)); !reflect.DeepEqual(got, []string{"github.com/yaklang/yaklang/common/utils/appconfig"}) {
		t.Fatalf("appconfig acquired non-standard dependencies: %v", got)
	}
}
