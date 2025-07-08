package pipeline_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/pipeline"
)

func TestPipe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	handler := func(item int) (string, error) {
		log.Infof("processing item: %d", item)
		if item%2 != 0 {
			return "", fmt.Errorf("odd number")
		}
		return strconv.Itoa(item * 2), nil
	}

	p := pipeline.NewPipe(ctx, 10, handler)

	go func() {
		for i := 0; i < 10; i++ {
			p.Feed(i)
		}
		p.Close()
	}()

	var results []string
	for result := range p.Out() {
		results = append(results, result)
	}

	assert.ElementsMatch(t, []string{"0", "4", "8", "12", "16"}, results)
}

func TestChainedPipe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var results2 []int
	// Pipe 1: item + 1
	pipe1 := pipeline.NewPipe(ctx, 10, func(item int) (int, error) {
		log.Infof("pipe1 processing item: %d", item)
		return item + 1, nil
	})

	// Pipe 2: item + 2
	pipe2 := pipeline.NewPipe(ctx, 10, func(item int) (int, error) {
		log.Infof("pipe2 processing item: %d", item)
		return item + 2, nil
	})

	// Start Pipe 1 with a slice
	pipe1.FeedSlice([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})

	// Start Pipe 2 with the output of Pipe 1
	pipe2.FeedChannel(pipe1.Out())

	for res := range pipe2.Out() {
		log.Infof("received %d from pipe2", res)
		results2 = append(results2, res)
	}
	log.Infof("pipe2 out closed")

	// assert.ElementsMatch(t, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, results1)
	assert.ElementsMatch(t, []int{3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, results2)
}
