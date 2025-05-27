package chunkmaker

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"math/rand"
	"testing"
	"time"
)

type basicItemInterface interface {
	String() string
}

type basicItem struct {
	msg string
}

func (b *basicItem) String() string {
	return b.msg
}

func generateMsg() basicItemInterface {
	return &basicItem{
		msg: fmt.Sprintf("now: %v, this message is in chanx unlimited chan: %v\n", time.Now(), utils.RandStringBytes(rand.Intn(400)+20)),
	}
}

func mockDatasource() *chanx.UnlimitedChan[basicItemInterface] {
	c := chanx.NewUnlimitedChan[basicItemInterface](context.Background(), 1000)
	go func() {

		for {
			time.Sleep(time.Duration(50+rand.Intn(700)) * time.Millisecond)
			msg := generateMsg()
			c.SafeFeed(msg)
		}
	}()
	return c
}

func TestChunker(t *testing.T) {
	// TDD mode
	src := mockDatasource()

}
