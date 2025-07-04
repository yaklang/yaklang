// Package tools
// @Author bcy2007  2025/5/12 14:54
package tools

import (
	"reflect"
	"testing"
)

func TestDynamicQueue(t *testing.T) {
	q := NewDynamicQueue()
	q.Enqueue("1", "2", "3")
	q.Range(func(item string, i int) bool {
		//time.Sleep(time.Second * 1)
		if item == "2" {
			q.Enqueue("4")
		}
		if item == "3" {
			q.Prepend(i, "5")
		}
		if item == "4" {
			q.Prepend(i, "99", "102")
		}
		if item == "99" {
			q.Prepend(i, "100", "102")
		}
		return true
	})
	expected := []string{"1", "2", "3", "5", "4", "99", "100", "102"}

	if !reflect.DeepEqual(q.items, expected) {
		t.Errorf(`identifier detect = %v, want %v`, q.items, expected)
	}
}
