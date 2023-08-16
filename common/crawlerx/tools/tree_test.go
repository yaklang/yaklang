// Package tools
// @Author bcy2007  2023/8/2 11:00
package tools

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTree(t *testing.T) {
	test := assert.New(t)
	tree := CreateTree("urlTestA.com")
	tree.Add("urlTestA.com", "urlTestB.com", "urlTestC.com")
	tree.Add("urlTestC.com", "urlTestCA.com", "urlTestCB.com")
	tree.Add("urlTestX.com", "urlTestXX.com")
	//t.Log(tree.Show())
	//t.Log(tree.Count(), tree.Level())
	test.Equal("urlTestA.com -> urlTestB.com\n"+
		"urlTestA.com -> urlTestC.com\n"+
		"urlTestA.com -> urlTestX.com\n"+
		"urlTestC.com -> urlTestCA.com\n"+
		"urlTestC.com -> urlTestCB.com\n"+
		"urlTestX.com -> urlTestXX.com\n", tree.Show())
	test.Equal(7, tree.Count())
	test.Equal(3, tree.Level())
}
