package ssatest

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestFrontendLuaRustKotlinScalaCompile covers the SSA lowering of the added
// frontends, including C++. Grammar-only parsing lives in the TestAllSyntax*_G4 drivers
// under the language tests packages and runs in the syntax-g4 workflow.
func TestFrontendLuaRustKotlinScalaCompile(t *testing.T) {
	cases := []struct {
		name string
		lang ssaconfig.Language
		file string
		code string
		refs []string
	}{
		{
			name: "lua",
			lang: ssaconfig.LUA,
			file: "main.lua",
			code: `
local function add(a, b)
    return a + b
end

function main()
    local x = 1
    x = add(x, 2)
    if x > 2 then
        x = x - 1
    end
    while x < 10 do
        x = x + 1
    end
    println(x)
end

main()
`,
			refs: []string{"add", "main", "x"},
		},
		{
			name: "rust",
			lang: ssaconfig.RUST,
			file: "main.rs",
			code: `
fn add(a: i32, b: i32) -> i32 {
    let c = a + b;
    return c;
}

fn main() {
    let mut x = 1;
    x = add(x, 2);
    if x > 2 {
        x = x - 1;
    }
    while x < 10 {
        x = x + 1;
    }
    println!("{}", x);
}
`,
			refs: []string{"add", "main", "x"},
		},
		{
			name: "kotlin",
			lang: ssaconfig.KOTLIN,
			file: "main.kt",
			code: `
package com.example

fun add(a: Int, b: Int): Int {
    return a + b
}

fun main() {
    var x = 1
    x = add(x, 2)
    if (x > 2) {
        println(x)
    }
    while (x < 10) {
        x += 1
    }
}
`,
			refs: []string{"add", "main"},
		},
		{
			name: "scala",
			lang: ssaconfig.SCALA,
			file: "Main.scala",
			code: `
package com.example

object Main {
  def add(a: Int, b: Int): Int = {
    a + b
  }

  def main(args: Array[String]): Unit = {
    var x: Int = 1
    x = add(x, 2)
    if (x > 2) {
      println(x)
    } else {
      println(0)
    }
    while (x < 10) {
      x = x + 1
    }
  }
}
`,
			refs: []string{"add", "main"},
		},
		{
			name: "cpp",
			lang: ssaconfig.CPP,
			file: "main.cpp",
			code: `
class Counter {
public:
    Counter() : total_(0) {}
    void add(int value) { total_ += value; }
    int total() const { return total_; }
private:
    int total_;
};

int main() {
    Counter counter;
    for (int i = 0; i < 3; i++) {
        counter.add(i);
    }
    return counter.total();
}
`,
			refs: []string{"Counter", "main", "counter"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vf := filesys.NewVirtualFs()
			vf.AddFile(tc.file, tc.code)

			progs, err := ssaapi.ParseProjectWithFS(
				vf,
				ssaapi.WithLanguage(tc.lang),
				ssaapi.WithMemory(),
			)
			require.NoError(t, err)
			require.NotEmpty(t, progs)

			prog := progs[0]
			for _, ref := range tc.refs {
				require.NotEmpty(t, prog.Ref(ref), "missing SSA symbol %q", ref)
			}
		})
	}
}
