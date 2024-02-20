package httptpl

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

// TestYakVariables_ToMap check circular reference
func TestYakVariables_ToMap(t *testing.T) {
	vars := NewVars()
	vars.SetAsNucleiTags("test", "{{test}}")
	vars.SetAsNucleiTags("a", "{{b}}")
	vars.SetAsNucleiTags("b", "{{a}}")
	res := vars.ToMap() // toMap occurs error
	assert.Equal(t, res["test"], "{{test}}")
	assert.Equal(t, res["a"], res["b"]) // var a and b value should be "{{a}}" or "{{b}}"
}
