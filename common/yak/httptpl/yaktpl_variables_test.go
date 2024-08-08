package httptpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestYakVariables_ToMap check circular reference
func TestYakVariables_ToMap(t *testing.T) {
	vars := NewVars()
	vars.SetAsNucleiTags("test", "{{test}}")
	vars.SetAsNucleiTags("a", "{{b}}")
	vars.SetAsNucleiTags("b", "{{a}}")
	res := vars.ToMap() // toMap occurs error
	assert.Equal(t, "{{test}}", res["test"])
	assert.Equal(t, res["a"], res["b"]) // var a and b value should be "{{a}}" or "{{b}}"
}
