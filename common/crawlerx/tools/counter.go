// Package tools
// @Author bcy2007  2023/7/12 16:40
package tools

import (
	"github.com/yaklang/yaklang/common/log"
	"sync"
	"time"
)

type Counter struct {
	sync.Mutex

	currentNumber int
	maxNumber     int
}

func NewCounter(number int) *Counter {
	c := Counter{
		maxNumber:     number,
		currentNumber: 0,
	}
	return &c
}

func (c *Counter) Number() int {
	return c.currentNumber
}

func (c *Counter) Add() bool {
	c.Lock()
	defer c.Unlock()
	if c.currentNumber >= c.maxNumber {
		return false
	}
	c.currentNumber += 1
	log.Infof(`after counter add: %d`, c.currentNumber)
	return true
}

func (c *Counter) Minus() bool {
	c.Lock()
	defer c.Unlock()
	if c.currentNumber == 0 {
		return false
	}
	c.currentNumber -= 1
	log.Infof(`after counter minus: %d`, c.currentNumber)
	return true
}

func (c *Counter) Wait(num int) {
	for true {
		if c.compare(num) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (c *Counter) compare(num int) bool {
	c.Lock()
	defer c.Unlock()
	if c.currentNumber >= num {
		return false
	}
	return true
}

func (c *Counter) OverLoad() bool {
	c.Lock()
	defer c.Unlock()
	return c.currentNumber >= c.maxNumber
}

func (c *Counter) LayDown() bool {
	c.Lock()
	defer c.Unlock()
	return c.currentNumber == 0
}
