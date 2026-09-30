package yakcmds

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	cli "github.com/yaklang/yaklang/common/urfavecli"
)

func TestYakFormatterCLI(t *testing.T) {
	run := func(args ...string) (string, error) {
		app := cli.NewApp()
		var output bytes.Buffer
		app.Writer = &output
		for _, command := range UtilsCommands {
			if command.Name == "fmt" {
				app.Commands = []cli.Command{*command}
				break
			}
		}
		if len(app.Commands) == 0 {
			t.Fatal("missing fmt command")
		}
		err := app.Run(append([]string{"yak", "fmt"}, args...))
		return output.String(), err
	}
	path := filepath.Join(t.TempDir(), "format.yak")
	if err := os.WriteFile(path, []byte(`include "missing.yak";func(a){if(xx){return xxx}}`), 0600); err != nil {
		t.Fatal(err)
	}
	want := "include \"missing.yak\"\nfunc(a) {\n    if (xx) {\n        return xxx\n    }\n}\n"
	if got, err := run(path); err != nil || got != want {
		t.Fatalf("got %q, %v", got, err)
	}
	if err := os.WriteFile(path, []byte("a="), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{}, {path}, {path + "missing"}} {
		if got, err := run(args...); err == nil || got != "" {
			t.Fatalf("invalid fmt invocation: %q %v", got, err)
		}
	}
	if got, err := run("--version"); err != nil || got != "Formatter version: 0.2.0\n" {
		t.Fatal(got, err)
	}
}
