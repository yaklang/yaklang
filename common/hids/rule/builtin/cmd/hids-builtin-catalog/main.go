//go:build hids

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/hids/rule/builtin"
)

func main() {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(builtin.DescribeRuleSets()); err != nil {
		fmt.Fprintf(os.Stderr, "encode hids builtin catalog: %v\n", err)
		os.Exit(1)
	}
}
