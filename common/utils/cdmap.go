package utils

import (
	"errors"
	"fmt"
	"github.com/tevino/abool"
	"sync"
	"time"
)

type CoolDownFetcher struct {
	mutex   *sync.Mutex
	timeout time.Duration
	cache   *sync.Map
}

func NewCoolDownFetcher(timeoutDuration time.Duration) *CoolDownFetcher {
	if timeoutDuration <= 100 {
		timeoutDuration = time.Second * 10
	}
	return &CoolDownFetcher{
		mutex:   new(sync.Mutex),
		cache:   new(sync.Map),
		timeout: timeoutDuration,
	}
}

var ErrCoolDownSkipCache = Error("Skip cache")

func (c *CoolDownFetcher) Fetch(handler func() (any, error)) (any, error) {
	cachedRet := func() (any, error) {
		result, ok := c.cache.Load("result")
		if ok {
			return result, nil
		}
		err, ok := c.cache.Load("error")
		if ok {
			return nil, Errorf("Fetch error: %s", err)
		}
		return nil, ErrCoolDownSkipCache
	}

	result, err := cachedRet()
	if err == nil || !errors.Is(err, ErrCoolDownSkipCache) {
		return result, err
	}

	c.mutex.Lock()
	handledExecuted := abool.New()
	defer func() {
		if handledExecuted.IsSet() {
			fmt.Println("wait cleaning cache")
			go func() {
				time.Sleep(c.timeout)
				fmt.Println("clearing cache")
				c.cache.Delete("result")
				c.cache.Delete("error")
			}()
		}
		c.mutex.Unlock()
	}()

	result, err = cachedRet()
	if err == nil || !errors.Is(err, ErrCoolDownSkipCache) {
		return result, err
	}

	result, errIns := handler()
	handledExecuted = abool.NewBool(true)
	if errIns != nil {
		c.cache.Store("error", errIns)
		return nil, errIns
	} else {
		c.cache.Store("result", result)
		return result, nil
	}
}
