package main

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
)

func main() {
	// Test the schema generation
	schema := aireact.GetYaklangCodeLoopSchema()
	fmt.Println("Generated Yaklang Code Loop Schema:")
	fmt.Println(schema)
}
