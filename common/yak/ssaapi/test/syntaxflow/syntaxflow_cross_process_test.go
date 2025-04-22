package syntaxflow

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestMultiDataFlow_CrossProcess(t *testing.T) {
	t.Run("test multi data flow cross process simple", func(t *testing.T) {
		code := `
		f= (param) => {
			return param	
		}
		d = f(1) + f(2)
     `
		ssatest.CheckSyntaxFlow(t, code, `d #-> * as $target`, map[string][]string{"target": {"1", "2"}})
	})

	t.Run("test multi data flow cross process with call as object", func(t *testing.T) {
		code := `
		f= () => {
			return {"b":1, "c":2}	
		}
		a=f()
		d = a.b + a.c
     `
		ssatest.CheckSyntaxFlow(t, code, `d #-> * as $target`, map[string][]string{"target": {"1", "2"}})
	})
}
