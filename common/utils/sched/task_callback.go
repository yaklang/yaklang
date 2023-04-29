package sched

import (
	"github.com/pkg/errors"
	"sync"
)

var (
	callbackLock = new(sync.Mutex)
)

type TaskCallback func(t *Task)

func (t *Task) OnFinished(tag string, callback TaskCallback) error {
	callbackLock.Lock()
	defer callbackLock.Unlock()

	if _, ok := t.onFinished[tag]; ok {
		return errors.Errorf("existed tag: %s in on_finished callbacks", tag)
	}

	t.onFinished[tag] = callback
	return nil
}

func (t *Task) OnBeforeExecuting(tag string, callback TaskCallback) error {
	callbackLock.Lock()
	defer callbackLock.Unlock()

	if _, ok := t.onBeforeExecuting[tag]; ok {
		return errors.Errorf("existed tag: %s in on_before_executing callbacks", tag)
	}

	t.onBeforeExecuting[tag] = callback
	return nil
}

func (t *Task) OnCanceled(tag string, callback TaskCallback) error {
	callbackLock.Lock()
	defer callbackLock.Unlock()

	if _, ok := t.onCanceled[tag]; ok {
		return errors.Errorf("existed tag: %s in on_cancel callbacks", tag)
	}

	t.onCanceled[tag] = callback
	return nil
}

func (t *Task) OnScheduleStart(tag string, callback TaskCallback) error {
	callbackLock.Lock()
	defer callbackLock.Unlock()

	if _, ok := t.onScheduleStart[tag]; ok {
		return errors.Errorf("existed tag: %s in on_working callbacks", tag)
	}

	t.onScheduleStart[tag] = callback
	return nil
}

func (t *Task) OnEveryExecuted(tag string, callback TaskCallback) error {
	callbackLock.Lock()
	defer callbackLock.Unlock()

	if _, ok := t.onEveryExecuted[tag]; ok {
		return errors.Errorf("existed tag: %s in on_every_executed callbacks", tag)
	}

	t.onEveryExecuted[tag] = callback
	return nil
}
