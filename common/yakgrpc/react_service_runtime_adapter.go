package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/sessionruntime"
	"github.com/yaklang/yaklang/common/ai/aid/reactservice"
	"github.com/yaklang/yaklang/common/consts"
)

func (s *Server) getReActSessionRuntime() sessionruntime.ReActSessionRuntime {
	if s == nil {
		return nil
	}
	return s.getReActService().Runtime()
}

func (s *Server) getReActService() *reactservice.Service {
	if s == nil {
		return nil
	}
	s.reActServiceMu.Lock()
	defer s.reActServiceMu.Unlock()
	if s.reActService == nil {
		if s.projectDatabase != nil {
			// Embedded servers with an explicitly injected database keep an
			// isolated service; the normal engine Server uses the process singleton.
			s.reActService = reactservice.New(s.GetProjectDatabase)
		} else {
			s.reActService = reactservice.Default()
		}
	}
	return s.reActService
}

// retireReActSessionRuntime stops every session owned by the current runtime and
// permanently quiesces that runtime before it is detached from the server. This
// is used when the project database changes: a ReAct created for the old project
// must never be attached through the new project context.
func (s *Server) retireReActSessionRuntime(ctx context.Context) error {
	if s == nil {
		return nil
	}
	return s.getReActService().RetireCurrentProject(ctx)
}

func (s *Server) resetReActSessionRuntimeAfterProjectSwitch() {
	if s == nil {
		return
	}
	projectDB := s.GetProjectDatabase
	if s.projectDatabase == nil {
		// The process service must not retain a transport Server merely to find
		// the current project. consts owns that durable process-level binding.
		projectDB = consts.GetGormProjectDatabase
	}
	s.getReActService().BindProject(projectDB)
}
