package tests

import (
	"fmt"
	"testing"
)

func TestName(t *testing.T) {
}

func printSlice(s []int) {
	fmt.Printf("len=%d cap=%d %v\n", len(s), cap(s), s)
}
