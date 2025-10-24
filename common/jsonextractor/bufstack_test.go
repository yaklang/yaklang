package jsonextractor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
)

func TestBufStack(t *testing.T) {
	count := 0
	buf := newBufStackManager(func(key any, val any, parent []string) {
		count++
		log.Infof("emit: %#v, %#v", key, val)
	})
	buf.PushKey("abc")
	buf.PushValue("value")
	buf.PushKey("abc")
	buf.PushValue("value")
	buf.PushKey("abc")
	buf.PushValue("value")
	buf.PushKey("abc")
	buf.PushValue("value")
	buf.PushKey("abc")
	buf.PushValue("value")
	assert.Equal(t, count, 5)
}

func TestBufStackContainerSimple(t *testing.T) {
	count := 0
	containerBasicPass := false
	buf := newBufStackManager(func(key any, val any, parent []string) {
		count++
		if key == "container" {
			if val, ok := val.(map[string]any); ok {
				assert.Equal(t, val["k"], "v")
			}
			containerBasicPass = true
		}
		log.Infof("emit: %#v, %#v", key, val)
	})
	buf.PushKey("abc")
	buf.PushKey("container")
	buf.PushContainer()
	buf.PushKey("k")
	buf.PushValue("v")
	buf.PopContainer()
	assert.Equal(t, count, 2)
	assert.True(t, containerBasicPass)
}

func TestBufStackContainer(t *testing.T) {
	count := 0
	buf := newBufStackManager(func(key any, val any, parent []string) {
		count++
		log.Infof("emit: %#v, %#v", key, val)
	})
	buf.PushKey("abc")
	buf.PushValue("value")
	buf.PushKey("c1")
	buf.PushContainer()
	buf.PushKey("abc")
	buf.PushValue("value")
	buf.PushKey("abc")
	buf.PushValue("value")
	buf.PushKey("abc")
	buf.PushValue("value")
	buf.PopContainer()
	assert.Equal(t, count, 5)
}

func TestBufStackContainer2(t *testing.T) {
	count := 0
	buf := newBufStackManager(func(key any, val any, parent []string) {
		count++
		log.Infof("emit: %#v, %#v", key, val)
	})
	buf.PushKey("outer")
	buf.PushValue("value")
	buf.PushKey("container")
	buf.PushContainer()
	buf.PushKey("inner1")
	buf.PushValue("value")
	buf.PushKey("container2")
	buf.PushContainer()
	buf.PushKey("inner1_1")
	buf.PushValue("value")
	buf.PopContainer()
	buf.PopContainer()
	buf.TriggerEmit()
	assert.Equal(t, count, 6)
}

func TestBufStackComplexKey(t *testing.T) {
	count := 0
	buf := newBufStackManager(func(key any, val any, parent []string) {
		count++
		log.Infof("emit: %#v, %#v", key, val)
	})
	buf.PushKey("outer")
	buf.PushValue("value")
	buf.PushKey("container")
	buf.PushContainer()
	buf.PushKey("inner1")
	buf.PushValue("value")
	buf.PushKey("container2")
	buf.PushContainer()
	buf.PushKey(0)
	buf.PushValue("value")
	buf.PopContainer()
	buf.PopContainer()
	buf.TriggerEmit()
	assert.Equal(t, count, 6)
}
