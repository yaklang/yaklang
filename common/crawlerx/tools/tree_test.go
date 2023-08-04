// Package tools
// @Author bcy2007  2023/8/2 11:00
package tools

import "testing"

func TestTree(t *testing.T) {
	tree := CreateTree("urlTestA.com")
	tree.Add("urlTestA.com", "urlTestB.com", "urlTestC.com")
	tree.Add("urlTestC.com", "urlTestCA.com", "urlTestCB.com")
	tree.Add("urlTestX.com", "urlTestXX.com")
	t.Log(tree.Show())
	t.Log(tree.Count(), tree.Level())
}
