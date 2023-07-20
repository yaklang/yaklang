package yaklib

import (
	"testing"
	"time"
)

func TestWaitGroup(t *testing.T) {
	t.Run("waitgroup-add-one", func(t *testing.T) {
		wg := NewWaitGroup()
		wg.Add()
		go func() {
			time.Sleep(time.Second)
			wg.Done()
		}()
		wg.Wait()
	})
	t.Run("waitgroup-add-two", func(t *testing.T) {
		wg := NewWaitGroup()
		wg.Add(2)
		go func() {
			time.Sleep(time.Second)
			wg.Done()
			time.Sleep(time.Second)
			wg.Done()
		}()
		wg.Wait()
	})
}

func TestSizedWaitGroup(t *testing.T) {
	t.Run("waitgroup-add-one", func(t *testing.T) {
		wg := NewSizedWaitGroup(10)
		wg.Add()
		go func() {
			time.Sleep(time.Second)
			wg.Done()
		}()
		wg.Wait()
	})
	t.Run("waitgroup-add-two", func(t *testing.T) {
		wg := NewSizedWaitGroup(10)
		wg.Add(2)
		go func() {
			time.Sleep(time.Second)
			wg.Done()
			time.Sleep(time.Second)
			wg.Done()
		}()
		wg.Wait()
	})
}
