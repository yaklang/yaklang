package wsm

import (
	"fmt"
	"testing"
)

func TestGenerator(t *testing.T) {
	generate := NewGenerate(
		WithPass("1"),
		WithAspxScript(),
		//WithSessionMode(),
		WithConfuse(),
	)
	s, err := generate.Generate()
	fmt.Println(s, err)
}
