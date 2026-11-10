//go:build !yakit_exclude

package yakgrpc

import (
	"database/sql"
	"sync"
	"sync/atomic"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

// Serialize persistence, subscription and publication so a proxy starting while
// a unary update is in flight cannot keep an older database snapshot. Scope the
// subscribers to the project connection, including cloned GORM handles.
var liveMITMFilters = struct {
	sync.Mutex
	byProject map[*sql.DB]map[*activeMITMFilter]struct{}
}{byProject: make(map[*sql.DB]map[*activeMITMFilter]struct{})}

type activeMITMFilter struct {
	atomic.Pointer[MITMFilter]
	projectDB *gorm.DB
	changed   chan struct{}
	onUpdate  func()
}

func newActiveMITMFilter(projectDB, profileDB *gorm.DB) (*activeMITMFilter, func()) {
	liveMITMFilters.Lock()
	defer liveMITMFilters.Unlock()

	f := &activeMITMFilter{projectDB: projectDB, changed: make(chan struct{}, 1)}
	f.Store(GetMITMFilterManager(projectDB, profileDB))
	key := projectDB.DB()
	if liveMITMFilters.byProject[key] == nil {
		liveMITMFilters.byProject[key] = make(map[*activeMITMFilter]struct{})
	}
	liveMITMFilters.byProject[key][f] = struct{}{}
	return f, func() {
		liveMITMFilters.Lock()
		defer liveMITMFilters.Unlock()
		delete(liveMITMFilters.byProject[key], f)
		if len(liveMITMFilters.byProject[key]) == 0 {
			delete(liveMITMFilters.byProject, key)
		}
	}
}

func (f *activeMITMFilter) setOnUpdate(fn func()) {
	liveMITMFilters.Lock()
	defer liveMITMFilters.Unlock()
	f.onUpdate = fn
}

// Stream response sanitization mutates filter data, so never send the published
// snapshot itself to the UI.
func (f *activeMITMFilter) GetData() *ypb.MITMFilterData {
	return cloneMITMFilterData(f.Load().Data)
}

func cloneMITMFilterData(data *ypb.MITMFilterData) *ypb.MITMFilterData {
	if data == nil {
		return nil
	}
	return proto.Clone(data).(*ypb.MITMFilterData)
}

func (f *activeMITMFilter) Update(data *ypb.MITMFilterData) error {
	_, err := saveMITMFilter(f.projectDB, data, f)
	return err
}

func saveMITMFilter(projectDB *gorm.DB, data *ypb.MITMFilterData, source *activeMITMFilter) (*ypb.MITMFilterData, error) {
	liveMITMFilters.Lock()
	defer liveMITMFilters.Unlock()

	// Published data and compiled matchers are immutable. Traffic readers can
	// finish using the previous snapshot without racing an in-place Update.
	data = cloneMITMFilterData(data)
	filter := NewMITMFilter(data)
	filter.db = projectDB
	if err := filter.SaveToDb(); err != nil {
		return nil, err
	}
	for subscriber := range liveMITMFilters.byProject[projectDB.DB()] {
		subscriber.Store(filter)
		if subscriber == source {
			// Stream callers already clear plugin caches and send their reply.
			continue
		}
		if subscriber.onUpdate != nil {
			subscriber.onUpdate()
		}
		// UI feedback must not hold up the unary RPC if a stream is not read.
		select {
		case subscriber.changed <- struct{}{}:
		default:
		}
	}
	return cloneMITMFilterData(data), nil
}
