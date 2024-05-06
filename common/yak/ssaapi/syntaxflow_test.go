package ssaapi

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestProgramSyntaxFlow_Match(t *testing.T) {
	t.Run("Test Match", func(t *testing.T) {
		prog, err := Parse(`
a = Runtime.getRuntime()
a.exec("bash attack")
`)
		if err != nil {
			t.Fatal(err)
		}

		results, err := prog.SyntaxFlowWithError(`.getRuntime`)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, 1, len(results))
		results.Show()
	})

	t.Run("Match MemberCallMember", func(t *testing.T) {
		prog, err := Parse(`
a = Runtime.getRuntime()
a.exec("bash attack")
`)
		if err != nil {
			t.Fatal(err)
		}

		results, err := prog.SyntaxFlowWithError(`.getRuntime().exec()`)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, 1, len(results))
		if !results[0].IsCall() {
			t.Fatal("expect call")
		}
	})

	t.Run("Match MemberCallMember", func(t *testing.T) {
		prog, err := Parse(`
a = Runtime.getRuntime()
a.exec("bash attack")
`)
		if err != nil {
			t.Fatal(err)
		}

		results, err := prog.SyntaxFlowWithError(`Runtime.getRuntime().exec()`)
		if err != nil {
			t.Fatal(err)
		}
		assert.Contains(t, results.String(), "Undefined-a.exec(valid)(\"bash attack\")")
		if !results[0].IsCall() {
			t.Fatal("expect call")
		}

	})

}
