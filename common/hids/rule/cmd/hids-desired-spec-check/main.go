//go:build hids

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/yaklang/yaklang/common/hids/model"
	hidsrule "github.com/yaklang/yaklang/common/hids/rule"
)

type checkOutput struct {
	Mode                string `json:"mode"`
	BuiltinRuleSetCount int    `json:"builtin_rule_set_count"`
	TemporaryRuleCount  int    `json:"temporary_rule_count"`
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "hids desired spec check failed: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("hids-desired-spec-check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	inputPath := flags.String("input", "-", "desired spec JSON path, or - for stdin")
	if err := flags.Parse(args); err != nil {
		return err
	}

	raw, err := readInput(*inputPath, stdin)
	if err != nil {
		return err
	}
	spec, err := model.ParseDesiredSpec(raw)
	if err != nil {
		return err
	}
	if _, err := hidsrule.NewEngine(spec); err != nil {
		return err
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(checkOutput{
		Mode:                spec.Mode,
		BuiltinRuleSetCount: len(spec.BuiltinRuleSets),
		TemporaryRuleCount:  len(spec.TemporaryRules),
	})
}

func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "" || path == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(path)
}
