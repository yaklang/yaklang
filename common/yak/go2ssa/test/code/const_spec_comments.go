//go:build ignore
// +build ignore

// Embedded comments in const/var/type declarations must not confuse the parser.
// Patterns from go/src/internal/types/testdata/check/constdecl.go and related syntax tests.
package main

const _ = 1     /* extra init expr 2 */
const _ int = 1 /* extra init expr 2 */

const (
	c0/* comment before type */ int = 0
	c1                              = /* comment before assign */ 1
	c2, c3                          = /* multi-name */ 2, 3
)

var _ = /* comment before assign */ 0

var _ /* comment before type */ int

var _ /* comment before typed assign */ int = 0

type alias = /* comment before alias assign */ int

type named /* comment before type body */ struct {
	x int
}

func constSpecComments() {
	_ = c0
	_ = c1
	_, _ = c2, c3
}
