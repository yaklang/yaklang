package java2ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDoWhileWithoutBracesParses(t *testing.T) {
	src := `class A {
  int f(byte[] b, int len) {
    int blen = b.length;
    if (len > blen) {
      do
        blen = blen * 2;
      while (len > blen);
      b = new byte[blen];
    }
    return blen;
  }
}`
	ast, err := Frontend(src)
	require.NoError(t, err, "valid Java do-while without braces must parse")
	_ = ast
}

func TestDoWhileWithBracesStillParses(t *testing.T) {
	src := `class B {
  void f(int n) {
    do { n--; } while (n > 0);
    do n++; while (n < 10);
  }
}`
	ast, err := Frontend(src)
	require.NoError(t, err)
	_ = ast
}
