package utils

import (
	"github.com/yaklang/yaklang/common/log"
	"testing"
)

func TestNewStringRoundRobinSelector(t *testing.T) {
	log.Info("start test string round robin")
	selector := NewStringRoundRobinSelector("a", "b", "c")

	var a, b, c string
	a = selector.Next()
	b = selector.Next()
	c = selector.Next()

	if a != b && a != c && b != c {
		return
	}

	t.Logf("1:%v 2:%v 3:%v", a, b, c)
	t.Fail()
}
