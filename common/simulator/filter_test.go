// Package simulator
// @Author bcy2007  2023/8/18 10:38
package simulator

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"golang.org/x/exp/maps"
	"testing"
)

func TestKeys(t *testing.T) {
	keys := maps.Keys(KeywordDict)
	t.Log(keys, funk.Keys(KeywordDict))
}

func TestSlices(t *testing.T) {
	a := make([][]float64, 0, 3)
	//a = append(a, []float64{1, 2, 3, 4})
	b := make([]float64, 0, 4)
	b = append(b, 1, 2, 3, 4)
	a = append(a, b)
	t.Logf(`%f`, a)
}
