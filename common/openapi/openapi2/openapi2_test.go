package openapi2_test

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/openapi/openapi2"
	"github.com/yaklang/yaklang/common/openapi/openapiyaml"
	"os"
	"reflect"
)

func Example() {
	input, err := os.ReadFile("testdata/swagger.json")
	if err != nil {
		panic(err)
	}

	var doc openapi2.T
	if err = json.Unmarshal(input, &doc); err != nil {
		panic(err)
	}
	if doc.ExternalDocs.Description != "Find out more about Swagger" {
		panic(`doc.ExternalDocs was parsed incorrectly!`)
	}

	outputJSON, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	var docAgainFromJSON openapi2.T
	if err = json.Unmarshal(outputJSON, &docAgainFromJSON); err != nil {
		panic(err)
	}
	if !reflect.DeepEqual(doc, docAgainFromJSON) {
		fmt.Println("objects doc & docAgainFromJSON should be the same")
	}

	outputYAML, err := openapiyaml.Marshal(doc)
	if err != nil {
		panic(err)
	}
	var docAgainFromYAML openapi2.T
	if err = openapiyaml.Unmarshal(outputYAML, &docAgainFromYAML); err != nil {
		panic(err)
	}
	if !reflect.DeepEqual(doc, docAgainFromYAML) {
		fmt.Println("objects doc & docAgainFromYAML should be the same")
	}

	// Output:
}
