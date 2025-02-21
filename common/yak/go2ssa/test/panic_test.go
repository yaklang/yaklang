package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Panic(t *testing.T) {
	t.Run("anonymous type panic in range next", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
	
			type A []int
	
			func (p *A) AcceptToken() (tok token.Token, ok bool) {
				for _, t := range p {
					if tok.Type == t {
						return tok, true
					}
				}
				return tok, false
			}
			`, []string{}, t)
	})

	t.Run("anonymous type panic in ExternMethodBuilder", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

			func (p *int) AcceptToken() (tok token.Token, ok bool) {
				tok = p.ReadToken()
				return tok, false
			}
			`, []string{}, t)
	})

}
