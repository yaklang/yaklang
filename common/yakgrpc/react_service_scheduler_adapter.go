package yakgrpc

import (
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireactscheduler"
	"github.com/yaklang/yaklang/common/schema"
)

const (
	aiReActScheduleTriggerSchedule = aireactscheduler.TriggerSchedule
	aiReActScheduleTriggerManual   = aireactscheduler.TriggerManual
)

// StartAIReActScheduler starts the project-scoped scheduler. Task definitions
// are durable, while active execution state intentionally remains in memory.
// It is safe to call more than once and is independent of UI streams.
func (s *Server) StartAIReActScheduler() {
	s.ensureAIReActScheduler()
}

func (s *Server) ensureAIReActScheduler() {
	if s == nil {
		return
	}
	s.getReActService().StartScheduler()
}

// StopAIReActScheduler stops the project-scoped scheduler and waits for its
// in-memory executions to release their resources.
func (s *Server) StopAIReActScheduler() {
	s.stopAIReActScheduler()
}

func (s *Server) stopAIReActScheduler() {
	if s == nil {
		return
	}
	s.getReActService().StopScheduler()
}

func (s *Server) wakeAIReActScheduler() {
	s.getReActService().WakeScheduler()
}

func (s *Server) enqueueAIReActSchedule(schedule *schema.AIReActSchedule, scheduledAt time.Time, trigger string) error {
	return s.getReActService().EnqueueSchedule(schedule, scheduledAt, trigger)
}

func (s *Server) cancelAIReActScheduleExecution(scheduleUUID string) {
	s.getReActService().CancelSchedule(scheduleUUID)
}
