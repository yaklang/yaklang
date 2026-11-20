package test

import (
	"testing"
)

func TestBashPrintlnBasic(t *testing.T) {
	for _, tc := range []struct {
		name string
		code string
		want []string
	}{
		{
			name: "assignment and quotes",
			code: `
x=hi
println "pre${x}post"
println '$x'
println ""
`,
			want: []string{`"prehipost"`, `"$x"`, `""`},
		},
		{
			name: "escaped words",
			code: `
println a\ b
println "a\"b"
println $'a\tb'
`,
			want: []string{`"a b"`, `"a\"b"`, `"a\tb"`},
		},
		{
			name: "empty and fallback",
			code: `
x=before
x=
println "${x:-after}"
println "${x-after}"
println "${x:=set}"
println "$x"
`,
			want: []string{`"after"`, `""`, `"set"`, `"set"`},
		},
		{
			name: "special parameters",
			code: `
set -- 'one two' '' three
println "$#"
println "$1"
println "$2"
println "$*"
shift
println "$#"
println "$2"
`,
			want: []string{`3`, `"one two"`, `""`, `"one two  three"`, `2`, `"three"`},
		},
		{
			name: "status",
			code: `
false
println "$?"
! false
println "$?"
x=alone
println "$?"
if false; then
  x=no
fi
println "$?"
f(){
  return 4
}
f
println "$?"
`,
			want: []string{`1`, `0`, `0`, `0`, `4`},
		},
		{
			name: "arrays and record fields",
			code: `
a=(zero one [4]=four)
a[1]=new
println "${a[0]}"
println "${a[1]}"
println "${a[4]}"
declare -A rec=([name]=blue [size]=wide)
rec[name]=green
println "${rec[name]}"
println "${rec[size]}"
`,
			want: []string{`"zero"`, `"new"`, `"four"`, `"green"`, `"wide"`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkBashPrintln(t, tc.code, tc.want...)
		})
	}
}
