package syntaxflow

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSF_Config_Until(t *testing.T) {
	t.Run("until", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		// match until 
		a = 11
		b1 = f(a,1)

		// no match until get undefined 
		b3 = ccc 
		`,
			"b* #{until:`* ?{opcode:call}`}-> * as $result",
			map[string][]string{
				"result": {"Undefined-f(11,1)"},
			})
	})

	t.Run("util in dataflow path", func(t *testing.T) {
		/*
			a
				-- f
					-- 1  // const
					-- 1  // actual-parameter
				-- f2
					-- 2  // const
					-- b //  actual-parameter  // only this path
		*/
		code := `
	f = (i) => {
		return i + 1
	}

	f2 = (i) => {
		return i + 2 
	}
	b = 11 
	a = f(1) + f2(b)
	`

		t.Run("test until contain include", func(t *testing.T) {
			ssatest.CheckSyntaxFlow(t, code, `
		b as $b
		a #{
			until: "* & $b"
		}-> as $output
		`, map[string][]string{
				"output": {"11"},
			})
		})
	})

}

func TestSF_Config_HOOK(t *testing.T) {
	t.Run("hook", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `
		a = 11
		b = f(a,1)
		`,
			"b #{hook:`* as $num`}-> as $result",
			map[string][]string{
				"num": {"Undefined-f(11,1)"},
			})
	})

}

func TestSF_Config_Exclude(t *testing.T) {
	t.Run("exclude in top value", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		// match exclude 
		b = f1(a1,1)

		// no match exclude get undefined
		b2 = f2(a2)
		`,
			"b* #{exclude:`* ?{opcode:const}`}-> as $result",
			map[string][]string{
				"result": {
					"Undefined-a1", "Undefined-f1",
					"Undefined-a2", "Undefined-f2",
				},
			})
	})

	t.Run("exclude in dataflow path ", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		b = f1(1 + d)

		b2 = 11 + c 
		`, "b* #{exclude: `* ?{opcode:call}`}-> as $result", map[string][]string{
			"result": {"Undefined-c", "11"},
		})
	})
}

func TestSF_Config_Include(t *testing.T) {
	t.Run("include", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		b0 = f1 + 0 
		b1 = f1(1)
		b2 = f2(2)
		b3 = f3(3)
		`,
			"b* #{include:`* ?{have:f1}`}-> as $result",
			map[string][]string{
				"result": {"Undefined-f1", "1", "0"},
			})
	})

	t.Run("include in dataflow path", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		b0 = f1 + 0 
		b1 = f1(1)
		b2 = f2(2)
		b3 = f3(3)
		`,
			"b* #{include:`* ?{have:f1 && opcode:call}`}-> as $result; ",
			map[string][]string{
				"result": {"Undefined-f1", "1"},
			})
	})
}

func TestSF_config_WithNameVariableInner(t *testing.T) {
	/*
		utils/include/exclude can use variable, but `__next__` is magic name,
		variable len:
			0:  just use `_` variable
			1:  use this  variable
			>1: use `__next__` variable
	*/
	check := func(t *testing.T, code string) {
		ssatest.CheckSyntaxFlow(t, `
		b0 = f1(1)

		b1 = f2 + 22
		`,
			code, map[string][]string{
				"result": {"Undefined-f1(1)"},
			})
	}
	t.Run("check no name", func(t *testing.T) {
		check(t, "b* #{until:`* ?{opcode:call}`}-> as $result")
	})

	t.Run("check only one name", func(t *testing.T) {
		check(t, "b* #{until:`* ?{opcode:call} as $name`}-> as $result")
	})

	t.Run("check only magic name", func(t *testing.T) {
		check(t, `
b* #{until: <<<UNTIL
	* ?{opcode:call} as $__next__
UNTIL
}-> as $result`)
	})

	t.Run("check mix magic name", func(t *testing.T) {
		check(t, `
b* #{until: <<<UNTIL
	* as $value;
	* ?{opcode:call} as $__next__
UNTIL
}-> as $result`)
	})
}

func TestSF_Config_MultipleConfig(t *testing.T) {
	code := `
f1 = () => {
	return 22
}

b = 11
if c1 {
	b = f1()
}else if c1 {
	b = f(b, 33)
}else {
	b = 44
}

println(b) // phi 
`
	t.Run("hook and exclude", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
println(* as $para);
$para #{
		hook: <<<HOOK
			*?{opcode:const} as $const
HOOK,
		exclude: <<<EXCLUDE
			*?{opcode:call}
EXCLUDE,
}-> as $result 
			`,
			map[string][]string{
				"const":  {"11", "22", "33", "44"},
				"result": {"44", "Undefined-c1"},
			})
	})
	t.Run("hook and until", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`
println(* as $para)
$para #{
	hook: <<<HOOK
			*?{opcode:const} as $const
HOOK,
	until: <<<UNTIL
		*?{opcode:call}
UNTIL,
}-> 

			`,
			map[string][]string{
				"const": {"44"},
			})
	})
}

func TestSF_NativeCall_DataFlow_DFS(t *testing.T) {
	code := `

/*
getCmd()
function-getCmd -> return(binaryOpAdd)
filter(param1) 
function_filter	-> return(binaryOpAdd)
parameter2 --> getFunction
actx pop call
...
*/
getCmd = (param1) => {
	return filter(param1) + "-al"
}

filter = (param2) => {
	return param2 - "-t" 
}

cmd = "ls"
if c1{
	cmd += "-l"
}else{
	cmd = getCmd()
}
exec(cmd)
`

	t.Run("exclude all paths", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
exec(* as $end);
$end #-> as $start;

$start<dataflow(
exclude:<<<EXCLUDE
	*?{have:'getCmd'}?{opcode:call}
EXCLUDE,
end:'end',
)>as $result;
			`,
			map[string][]string{
				"start":  {"\"-al\"", "\"-l\"", "\"-t\"", "\"ls\"", "Parameter-param1", "Undefined-c1"},
				"result": {"\"-l\"", "\"ls\"", "Undefined-c1"},
				"end":    {"phi(cmd)[\"ls-l\",Function-getCmd() binding[Function-filter]]"},
			})
	})

	t.Run("exclude some of paths", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
exec(* as $end);
$end #-> as $start;

$start<dataflow(
exclude:<<<EXCLUDE
	filter?{opcode:function}
EXCLUDE,
end:'end',
)>as $result;
			`,
			map[string][]string{
				"start":  {"\"-al\"", "\"-l\"", "\"-t\"", "\"ls\"", "Parameter-param1", "Undefined-c1"},
				"result": {"\"-al\"", "\"-l\"", "\"ls\"", "Parameter-param1", "Undefined-c1"},
				"end":    {"phi(cmd)[\"ls-l\",Function-getCmd() binding[Function-filter]]"},
			})
	})

	t.Run("include some of paths", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
exec(* #->as $start);
getCmd?{opcode:function} as $end;

$start<dataflow(
include:<<<INCLUDE
	*?{have:'-t'}
INCLUDE,
end:'end',
)>as $result;
			`,
			map[string][]string{
				"start":  {"\"-al\"", "\"-l\"", "\"-t\"", "\"ls\"", "Parameter-param1", "Undefined-c1"},
				"result": {"\"-t\"", "Parameter-param1"},
				"end":    {"Function-getCmd"},
			})
	})
}

func TestSF_Until_Real_Demo(t *testing.T) {
	t.Run("test until edge demo", func(t *testing.T) {
		code := ` 
package com.example;
	class Main{
    public R vul(@RequestParam("file") MultipartFile file, HttpServletRequest request) {
         String res;
        String suffix = file.getOriginalFilename().substring(file.getOriginalFilename().lastIndexOf(".") + 1);
        if (!uploadUtil.checkFileSuffixWhiteList(suffix)){
            return R.error("文件后缀不合法");
        }
        String path = request.getScheme() + "://" + request.getServerName() + ":" + request.getServerPort() + "/file/";
        target=file+ suffix+ path;
    }
}`

		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			vals, err := prog.SyntaxFlowWithError(`
MultipartFile?{opcode:param} as $source
target* #{until: <<<UNTIL
 * & $source
UNTIL
}-> as $result;
`)
			require.NoError(t, err)
			result := vals.GetValues("result")
			result.Show()
			require.Equal(t, 1, len(result))
			require.Equal(t, "Parameter-file", result[0].String())
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}
