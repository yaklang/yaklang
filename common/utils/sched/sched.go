package sched

import (
	"github.com/pkg/errors"
	"sync"
)

type Sched struct {
	// map[taskID: string]*Task{}
	tasks sync.Map
}

func (s *Sched) Feed(t *Task) error {
	_, loaded := s.tasks.Load(t.ID)
	if loaded {
		return errors.Errorf("task: %s failed for existed id: %s", t.ID, t.ID)
	}

	err := t.Execute()
	if err != nil {
		return errors.Errorf("task: %s execute failed: %s", t.ID, err)
	}

	s.tasks.Store(t.ID, t)
	return nil
}

func (s *Sched) GetTaskByID(id string) (*Task, error) {
	if raw, ok := s.tasks.Load(id); ok {
		return raw.(*Task), nil
	}
	return nil, errors.Errorf("no existed task-id: %s", id)
}

func (s *Sched) ForeachTask(f func(task *Task) bool) {
	s.tasks.Range(func(key, value interface{}) bool {
		task := value.(*Task)
		return f(task)
	})
}
