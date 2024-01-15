package test

import "testing"

func TestYakBuildInMethod(t *testing.T) {
	t.Run("slice insert", func(t *testing.T) {
		check(t, `
		a = [] 
		a.Insert(0, 1)
		`, []string{})
	})
}
