package sched

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/tevino/abool"
	"testing"
	"time"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
)

func TestTask(t *testing.T) {
	var count int
	task := NewTask(1*time.Second, "test", time.Now().Add(-1), time.Now().Add(-1), func() {
		count++
	}, true)

	c, _ := context.WithTimeout(context.Background(), 3400*time.Millisecond)
	err := task.ExecuteWithContext(c)
	if err != nil {
		t.Errorf("execute failed: %s", err)
		t.FailNow()
	}

	select {
	case <-c.Done():
		if count != 4 {
			t.Errorf("execute failed: expect count: 4, actually: %v", count)
			t.FailNow()
		}
	}
}

func TestTask_WithEnd(t *testing.T) {
	var count int
	task := NewTask(1*time.Second, "test", time.Now().Add(-1), time.Now().Add(2500*time.Millisecond), func() {
		count++
	}, true)

	c, _ := context.WithTimeout(context.Background(), 3400*time.Millisecond)
	err := task.ExecuteWithContext(c)
	if err != nil {
		t.Errorf("execute failed: %s", err)
		t.FailNow()
	}

	select {
	case <-c.Done():
		if count != 3 {
			t.Errorf("execute failed: expect count: 4, actually: %v", count)
			t.FailNow()
		}
	}
}

func TestTask_WithStart(t *testing.T) {
	var count int
	task := NewTask(1*time.Second, "test", time.Now().Add(1100*time.Millisecond), time.Now().Add(-1), func() {
		count++
	}, true)

	c, _ := context.WithTimeout(context.Background(), 3400*time.Millisecond)
	err := task.ExecuteWithContext(c)
	if err != nil {
		t.Errorf("execute failed: %s", err)
		t.FailNow()
	}

	select {
	case <-c.Done():
		if count != 3 {
			t.Errorf("execute failed: expect count: 4, actually: %v", count)
			t.FailNow()
		}
	}
}

func TestTask_WithoutFirst(t *testing.T) {
	var count int
	task := NewTask(1*time.Second, "test", time.Now().Add(-1), time.Now().Add(-1), func() {
		count++
	}, false)

	c, _ := context.WithTimeout(context.Background(), 3400*time.Millisecond)
	err := task.ExecuteWithContext(c)
	if err != nil {
		t.Errorf("execute failed: %s", err)
		t.FailNow()
	}

	select {
	case <-c.Done():
		if count != 3 {
			t.Errorf("execute failed: expect count: 4, actually: %v", count)
			t.FailNow()
		}
	}
}

func TestTask_Hooks(t *testing.T) {
	var (
		executed        = abool.New()
		worked          = abool.New()
		finished        = abool.New()
		beforeExecuting = abool.New()
	)

	task := NewTask(1*time.Second, "test", time.Now().Add(-1), time.Now().Add(4*time.Second), func() {
		log.Info("hooks func")
	}, true)
	task.OnEveryExecuted("1", func(t *Task) {
		executed.Set()
	})

	task.OnFinished("2", func(t *Task) {
		finished.Set()
	})

	task.OnScheduleStart("1", func(t *Task) {
		worked.Set()
	})

	task.OnBeforeExecuting("1", func(t *Task) {
		beforeExecuting.Set()
	})

	ctx := utils.TimeoutContext(2 * time.Second)
	err := task.ExecuteWithContext(ctx)
	if err != nil {
		log.Infof("execute failed: %s", err)
		t.FailNow()
	}

	select {
	case <-ctx.Done():
	}

	test := assert.New(t)
	test.True(executed.IsSet())
	test.True(worked.IsSet())
	test.True(finished.IsSet())
	test.True(beforeExecuting.IsSet())

}
