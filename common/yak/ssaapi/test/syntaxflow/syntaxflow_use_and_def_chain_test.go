package syntaxflow

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func checkDirectlyConnect(t *testing.T, v1 ssaapi.Values, v2 *ssaapi.Value) {
	for _, v := range v1 {
		v.ForEachEffectOn(func(value *ssaapi.Value) {
			require.NotEqual(t, value.GetId(), v2.GetId())
		})
		v.ForEachDependOn(func(value *ssaapi.Value) {
			require.NotEqual(t, value.GetId(), v2.GetId())
		})
	}
}

func Test_TopDef_UD_Relationship(t *testing.T) {
	t.Run("test topdef ud chain: from formal param to actual param", func(t *testing.T) {
		code := `
			f2 := func(param2) {
				exec(param2)			
			}
		  	f1 := func(param1) {
				if !isValid(source) {
					return
				}
				f2(param1)
			}
		`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			rule := `
exec( * as $sink);
param1?{opcode:param} as $source;
$sink #-> as $result; 
$result<dataflow( <<<CODE
	<self> & $sink as $start;
	<self> & $source as $end;
CODE
)>
`
			vals, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			start := vals.GetValues("start")
			require.Contains(t, start.String(), "Parameter-param2")
			require.NotNil(t, start)

			end := vals.GetValues("end")
			require.NotNil(t, end)
			require.Contains(t, end.String(), "Parameter-param1")

			//start.ShowDot()
			checkDirectlyConnect(t, start, end[0])
			return nil
		})
	})

	t.Run("test topdef ud chain: from formal parma-member to actual param", func(t *testing.T) {
		code := `
			f2 := func(param2) {
				exec(param2.foo)			
			}
		  	f1 := func(param1) {
				if !isValid(param1) {
					return
				}
				f2(param1)
			}
		`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			rule := `
exec( * as $sink);
param1?{opcode:param} as $source;
$sink #-> as $result; 
$result<dataflow( <<<CODE
	<self> & $sink as $start;
	<self> & $source as $end;
CODE
)>
`
			vals, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			start := vals.GetValues("start")
			require.Contains(t, start.String(), "ParameterMember-parameter[0].foo")
			require.NotNil(t, start)

			end := vals.GetValues("end")
			require.NotNil(t, end)
			require.Contains(t, end.String(), "Parameter-param1")

			//start.ShowDot()
			checkDirectlyConnect(t, start, end[0])
			return nil
		})
	})

	t.Run("test topdef ud chain: from undefined-call to actual param", func(t *testing.T) {
		code := `
			f2 := func(param2) {
				exec(getCmd(param2))			
			}
		  	f1 := func(param1) {
				if !isValid(source) {
					return
				}
				f2(param1)
			}
		`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			rule := `
exec( * as $sink);
param1?{opcode:param} as $source;
$sink #-> as $result; 
$result<dataflow( <<<CODE
	<self> & $sink as $start;
	<self> & $source as $end;
CODE
)>
`
			vals, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			start := vals.GetValues("start")
			require.Contains(t, start.String(), "FreeValue-getCmd(Parameter-param2)")
			require.NotNil(t, start)

			end := vals.GetValues("end")
			require.NotNil(t, end)
			require.Contains(t, end.String(), "Parameter-param1")

			//start.ShowDot()
			checkDirectlyConnect(t, start, end[0])
			return nil
		})
	})

	t.Run("test topdef ud chain: from call to actual param", func(t *testing.T) {
		code := `
			getCmd:= func(param3){
				return "ls" + param3
			}
			f2 := func(param2) {
				exec(getCmd(param2))			
			}
		  	f1 := func(param1) {
				if !isValid(source) {
					return
				}
				f2(param1)
			}
		`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			rule := `
exec( * as $sink);
param1?{opcode:param} as $source;
$sink #-> as $result; 
$result<dataflow( <<<CODE
	<self> & $sink as $start;
	<self> & $source as $end;
CODE
)>
`
			vals, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			start := vals.GetValues("start")
			require.Contains(t, start.String(), "FreeValue-getCmd(Parameter-param2)")
			require.NotNil(t, start)

			end := vals.GetValues("end")
			require.NotNil(t, end)
			require.Contains(t, end.String(), "Parameter-param1")

			start.ShowDot()
			checkDirectlyConnect(t, start, end[0])
			return nil
		})
	})
}

func Test_Bottom_Use_UD_Relationship(t *testing.T) {
	t.Run("test bottom ud chain: return --> function", func(t *testing.T) {
		code := `
		f = () =>{
			a = 11
			return a
		}
		f2 = (i) => {
			println(i)
		}
		t = f()
		f2(t) 
`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			rule := `
a as $source;


$source -{
hook:<<<CODE
<self>?{opcode:return} as $start;
<self>?{opcode:call}?{have:'f2'} as $end;
CODE
}-> as $result;
			`
			vals, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			//result := vals.GetValues("result")
			//result.ShowDot()
			start := vals.GetValues("start")
			require.Contains(t, start.String(), "return(11)")
			require.NotNil(t, start)
			end := vals.GetValues("end")
			end.Show()
			require.NotNil(t, end)
			require.Contains(t, end.String(), "Function-f2(Function-f())")
			checkDirectlyConnect(t, start, end[0]) // f2(t)和return不应该直接连接
			return nil
		})
	})
}
