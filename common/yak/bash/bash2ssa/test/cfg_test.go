package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// Every case asserts the value printed after the control-flow merge. A Phi in
// the printed value means the frontend kept both reaching definitions.
func TestBashPhiConditions(t *testing.T) {
	for _, tc := range []struct {
		name string
		code string
		want string
	}{
		{
			name: "if else",
			code: `
if [ "$1" = a ]; then
  X=1
else
  X=2
fi
println "$X"
`,
			want: `^phi\(X\)\["1","2"\]$`,
		},
		{
			name: "if without else",
			code: `
X=0
if [ "$1" = a ]; then
  X=1
fi
println "$X"
`,
			want: `^phi\(X\)\["1","0"\]$`,
		},
		{
			name: "first assignment in both branches",
			code: `
if [ "$1" = a ]; then
  Y=1
else
  Y=2
fi
println "$Y"
`,
			want: `^phi\(Y\)\["1","2"\]$`,
		},
		{
			name: "elif chain",
			code: `
if [ "$1" = a ]; then
  X=1
elif [ "$1" = b ]; then
  X=2
else
  X=3
fi
println "$X"
`,
			want: `^phi\(X\)\["1","2","3"\]$`,
		},
		{
			name: "case",
			code: `
case "$1" in
  a) X=1 ;;
  b) X=2 ;;
esac
println "$X"
`,
			want: `^phi\(X\)\["1","2",Undefined-X\]$`,
		},
		{
			name: "short circuit and",
			code: `
A=1
true && A=2
println "$A"
`,
			want: `^phi\(A\)\["2","1"\]$`,
		},
		{
			name: "short circuit or",
			code: `
A=1
false || A=3
println "$A"
`,
			want: `^phi\(A\)\["3","1"\]$`,
		},
		{
			name: "command substitution",
			code: `
X=$(if true; then echo 1; else echo 2; fi)
println "$X"
`,
			want: `^phi\(X\)\["1","2"\]$`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkBashPrintlnRegexp(t, tc.code, tc.want)
		})
	}
}

func TestBashPhiLoops(t *testing.T) {
	for _, tc := range []struct {
		name string
		code string
		want string
	}{
		{
			name: "word list for",
			code: `
for i in a b; do
  X=$i
done
println "$X"
`,
			want: `^phi\(X\)\[Undefined-X,phi\(X\)\["a","b"\]\]$`,
		},
		{
			name: "c style for",
			code: `
for ((i=0; i<3; i++)); do
  Y=$i
done
println "$Y"
`,
			want: `^phi\(Y\)\[Undefined-Y,phi\(Y\)\[0,add\(.*\)\]\]$`,
		},
		{
			name: "while",
			code: `
X=0
while read -r line; do
  X=$line
done < /tmp/f
println "$X"
`,
			want: `^phi\(X\)\["0",Undefined-read\("read","-r","line"\) binding\["/tmp/f"\]\]$`,
		},
		{
			name: "until",
			code: `
N=0
until [ "$N" -gt 3 ]; do
  N=$((N+1))
done
println "$N"
`,
			want: `^phi\(N\)\["0",castType\(string, add\(.*\)\)\]$`,
		},
		{
			name: "break keeps the body value",
			code: `
while true; do
  X=1
  break
done
println "$X"
`,
			want: `^phi\(X\)\["1",phi\(X\)\[Undefined-X,"1"\]\]$`,
		},
		{
			name: "nested loop in if",
			code: `
if true; then
  for i in a b; do
    X=$i
  done
fi
println "$X"
`,
			want: `^phi\(X\)\[phi\(X\)\[Undefined-X,phi\(X\)\["a","b"\]\],Undefined-X\]$`,
		},
		{
			name: "select",
			code: `
select choice in a b; do
  X=$choice
  break
done
println "$X"
`,
			want: `^phi\(X\)\[Undefined-@bash\.select\.item\(`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkBashPrintlnRegexp(t, tc.code, tc.want)
		})
	}
}

// A literal for list is unrolled so each iteration keeps its own call and the
// final printed value is concrete instead of an opaque loop Phi.
func TestBashLiteralForUnrolling(t *testing.T) {
	checkBashPrintln(t, `
for i in a b; do
  println "$i"
done
`, `"a"`, `"b"`)

	checkBashPrintln(t, `
i=old
for i in; do
  i=bad
done
println "$i"
for i in a b; do
  :
done
println "$i"
`, `"old"`, `"b"`)
}

func TestBashBreakDepth(t *testing.T) {
	checkBashPrintlnRegexp(t, `
for i in a; do
  for j in b; do
    X=$i$j
    break 2
  done
done
println "$X"
`, `^phi\(X\)\[add\(.*\),phi\(X\)\[Undefined-X,add\(.*\)\]\]$`)
}

func TestBashContinueSkipsRestOfBody(t *testing.T) {
	checkBash(t, `
for i in a b; do
  continue
  never
done
`, func(prog *ssaapi.Program) {
		requireNoCall(t, prog, "never")
	})
}

func TestBashBackgroundJob(t *testing.T) {
	checkBashPrintln(t, `
sleep 1 &
wait
X=done
println "$X"
`, `"done"`)
}
