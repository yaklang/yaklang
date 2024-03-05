package utils

import (
	"context"
	"testing"
)

func TestSizedWaitGroup_AddWithContext(t *testing.T) {
	swg := NewSizedWaitGroup(10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 10000; i++ {
		err := swg.AddWithContext(ctx)
		if err == nil {
			swg.Done()
			t.Fatal("Smoking test AddWithContext fail")
		}
	}
}
