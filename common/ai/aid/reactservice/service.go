package reactservice

import (
	"context"
	"sync"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/sessionruntime"
	"github.com/yaklang/yaklang/common/ai/aid/aireactscheduler"
	"github.com/yaklang/yaklang/common/ai/aid/aischedule"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// Service owns the process-level ReAct entry point and the scheduler for the
// currently bound project. Transports are adapters: they may retain a Service
// reference, but they do not own Session Runtime or Scheduler lifecycle.
//
// A project switch retires the current scope before BindProject installs a new
// one. The retired Runtime remains quiesced so stale references cannot create a
// ReAct against the old project database.
type Service struct {
	// lifecycleMu serializes project-scope transitions. In particular, a
	// concurrent StartScheduler cannot recreate the old project's scheduler in
	// the gap between stopping it and quiescing its Runtime.
	lifecycleMu sync.Mutex
	mu          sync.Mutex

	projectDB      func() *gorm.DB
	runtime        sessionruntime.ReActSessionRuntime
	runtimeRetired bool
	scheduler      *aireactscheduler.Scheduler
}

type Option func(*Service)

// WithRuntime injects a Session Runtime. It is intended for embedding and
// deterministic tests; production normally lets Service create the Runtime.
func WithRuntime(runtime sessionruntime.ReActSessionRuntime) Option {
	return func(service *Service) {
		service.runtime = runtime
	}
}

func New(projectDB func() *gorm.DB, options ...Option) *Service {
	service := &Service{projectDB: projectDB}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) runtimeLocked() sessionruntime.ReActSessionRuntime {
	if s.runtime == nil {
		s.runtime = sessionruntime.New(s.projectDB)
		s.runtimeRetired = false
	}
	return s.runtime
}

func (s *Service) Runtime() sessionruntime.ReActSessionRuntime {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runtimeLocked()
}

// StartScheduler starts the scheduler for the current project scope. Calls are
// idempotent until StopScheduler or RetireCurrentProject detaches that scope.
func (s *Service) StartScheduler() {
	if s == nil {
		return
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	if s.scheduler != nil {
		s.mu.Unlock()
		return
	}
	if s.runtimeRetired {
		s.mu.Unlock()
		return
	}
	runtime := s.runtimeLocked()
	db := s.projectDB
	var projectDB *gorm.DB
	if db != nil {
		projectDB = db()
	}
	scheduler := aireactscheduler.NewScheduler(runtime, projectDB)
	s.scheduler = scheduler
	s.mu.Unlock()
	scheduler.Start()
}

// StopScheduler prevents new scheduled work and waits for in-memory executions
// to finish their cancellation and cleanup.
func (s *Service) StopScheduler() {
	if s == nil {
		return
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.stopSchedulerLocked()
}

// stopSchedulerLocked requires lifecycleMu and may wait for active executions.
func (s *Service) stopSchedulerLocked() {
	s.mu.Lock()
	scheduler := s.scheduler
	s.scheduler = nil
	s.mu.Unlock()
	if scheduler != nil {
		scheduler.Stop()
	}
}

func (s *Service) WakeScheduler() {
	if s == nil {
		return
	}
	s.mu.Lock()
	scheduler := s.scheduler
	s.mu.Unlock()
	if scheduler != nil {
		scheduler.Notify()
	}
}

func (s *Service) EnqueueSchedule(schedule *schema.AIReActSchedule, scheduledAt time.Time, trigger string) error {
	if s == nil {
		return utils.Error("AI ReAct service is not configured")
	}
	s.mu.Lock()
	scheduler := s.scheduler
	s.mu.Unlock()
	if scheduler == nil {
		return utils.Error("AI ReAct scheduler is not running")
	}
	return scheduler.Enqueue(schedule, scheduledAt, trigger)
}

func (s *Service) CancelSchedule(scheduleUUID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	scheduler := s.scheduler
	s.mu.Unlock()
	if scheduler != nil {
		scheduler.CancelSchedule(scheduleUUID)
		return
	}
	aischedule.CancelExecution(scheduleUUID)
}

func (s *Service) ActiveExecution(scheduleUUID string) (aireactscheduler.ActiveExecution, bool) {
	if s == nil {
		return aireactscheduler.ActiveExecution{}, false
	}
	s.mu.Lock()
	scheduler := s.scheduler
	s.mu.Unlock()
	if scheduler == nil {
		return aireactscheduler.ActiveExecution{}, false
	}
	return scheduler.ActiveExecution(scheduleUUID)
}

// RetireCurrentProject stops scheduling, permanently quiesces the active
// Runtime, and keeps that Runtime installed until BindProject closes the
// project-switch window.
func (s *Service) RetireCurrentProject(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.stopSchedulerLocked()
	s.mu.Lock()
	if s.runtimeRetired {
		s.mu.Unlock()
		return nil
	}
	runtime := s.runtimeLocked()
	s.mu.Unlock()

	// Intentionally keep the returned guard: stale holders of this runtime must
	// continue to reject reservations and connections.
	if _, err := runtime.QuiesceAll(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	if s.runtime == runtime {
		s.runtimeRetired = true
	}
	s.mu.Unlock()
	return nil
}

// BindProject installs a fresh project scope after the caller has switched the
// durable database binding. It does not start the Scheduler automatically.
func (s *Service) BindProject(projectDB func() *gorm.DB) {
	if s == nil {
		return
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.stopSchedulerLocked()
	s.mu.Lock()
	s.projectDB = projectDB
	s.runtime = nil
	s.runtimeRetired = false
	s.mu.Unlock()
}

// Stop shuts down all project-scoped background execution owned by the service.
func (s *Service) Stop(ctx context.Context) error {
	return s.RetireCurrentProject(ctx)
}
