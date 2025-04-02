package yakurl

import (
	"context"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/wsm"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/yakurl/java_decompiler"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type Action interface {
	Get(*ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error)
	Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error)
	Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error)
	Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error)
	Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error)
	Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error)
}

type ActionService struct {
	actions map[string]Action
	mutex   sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
}

var (
	actionServiceInstance *ActionService
	once                  sync.Once
)

func GetActionService() *ActionService {
	once.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		actionServiceInstance = &ActionService{
			actions: make(map[string]Action),
			ctx:     ctx,
			cancel:  cancel,
		}
		go actionServiceInstance.clearCachePeriodically()
	})
	return actionServiceInstance
}

func (s *ActionService) Stop() {
	s.cancel()
}

func (s *ActionService) GetAction(schema string) Action {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	action, exists := s.actions[schema]
	if !exists {
		action = s.CreateAction(schema)
		if action != nil {
			s.actions[schema] = action
		}
	}
	return action
}

func (s *ActionService) CreateAction(schema string) Action {
	switch schema {
	case "file":
		return &fileSystemAction{
			fs: filesys.NewLocalFs(),
		}
	case "website":
		return &websiteFromHttpFlow{}
	case "behinder":
		return &wsm.BehidnerResourceSystemAction{}
	case "godzilla":
		return &wsm.GodzillaFileSystemAction{}
	case "fuzztag":
		return &fuzzTagDocAction{}
	case "yakdocument":
		return &documentAction{}
	case "facades":
		return newFacadeServerAction()
	case "yakshell":
		return &wsm.YakShellResourceAction{}
	case "syntaxflow":
		return NewSyntaxFlowAction()
	case "ssadb":
		return &fileSystemAction{
			fs: ssadb.NewIrSourceFs(),
		}
	case "ssarisk":
		return &riskTreeAction{
			register: make(map[string]int),
		}
	case "javadec":
		return java_decompiler.NewJavaDecompilerAction()
	default:
		return nil
	}
}

func (s *ActionService) clearCachePeriodically() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.clearCache()
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *ActionService) clearCache() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.actions = make(map[string]Action)
}
