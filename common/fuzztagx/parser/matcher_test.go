package parser

import "testing"

func TestMatch(t *testing.T) {
	RootRule.Match("{{int()}}")
}
