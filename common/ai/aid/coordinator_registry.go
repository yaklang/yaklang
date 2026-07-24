package aid

import (
	"sync"
)

var runningCoordinators sync.Map // coordinator id -> *Coordinator

func registerRunningCoordinator(c *Coordinator) {
	if c == nil || c.Config == nil {
		return
	}
	id := c.Config.Id
	if id == "" {
		return
	}
	runningCoordinators.Store(id, c)
}

func unregisterRunningCoordinator(id string) {
	if id == "" {
		return
	}
	runningCoordinators.Delete(id)
}

func snapshotRunningCoordinators() []*Coordinator {
	var coordinators []*Coordinator
	runningCoordinators.Range(func(key, value any) bool {
		c, ok := value.(*Coordinator)
		if !ok || c == nil {
			return true
		}
		coordinators = append(coordinators, c)
		return true
	})
	return coordinators
}

// GetRunningCoordinators returns a snapshot of all currently-running coordinators.
// 关键词: coordinator registry, live agents, snapshot
func GetRunningCoordinators() []*Coordinator {
	return snapshotRunningCoordinators()
}
