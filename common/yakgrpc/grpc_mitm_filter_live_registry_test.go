package yakgrpc

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func newLiveMITMFilterTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.ProjectGeneralStorage{}).Error)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestLiveMITMFilterProjectIsolationAndCleanup(t *testing.T) {
	db, otherDB := newLiveMITMFilterTestDB(t), newLiveMITMFilterTestDB(t)
	first, unregisterFirst := newActiveMITMFilter(db, nil)
	defer unregisterFirst()
	second, unregisterSecond := newActiveMITMFilter(db.New(), nil)
	defer unregisterSecond()
	other, unregisterOther := newActiveMITMFilter(otherDB, nil)
	defer unregisterOther()
	before, otherBefore := first.Load(), other.Load()
	var resets atomic.Int32
	second.setOnUpdate(func() { resets.Add(1) })
	data := &ypb.MITMFilterData{ExcludeUri: []*ypb.FilterDataItem{{MatcherType: "word", Group: []string{"/blocked"}}}}
	require.NoError(t, first.Update(data))
	require.NotSame(t, before, first.Load())
	require.True(t, before.IsPassed("GET", "example.test", "/blocked", ""), "old snapshots stay valid")
	require.False(t, second.Load().IsPassed("GET", "example.test", "/blocked", ""))
	require.Same(t, otherBefore, other.Load(), "another project's proxy must be unchanged")
	require.EqualValues(t, 1, resets.Load())
	require.Len(t, first.changed, 0, "stream callers send their own acknowledgement")
	require.Len(t, second.changed, 1)
	require.Len(t, other.changed, 0)
	data.ExcludeUri[0].Group[0] = "/mutated"
	feedback := first.GetData()
	feedback.ExcludeUri[0].Group[0] = "/sanitized"
	require.Equal(t, "/blocked", first.Load().Data.ExcludeUri[0].Group[0], "request data is not shared with traffic readers")

	// Updates must complete even if a stream has not consumed its notification.
	server := &Server{projectDatabase: db}
	_, err := server.ResetMITMFilter(context.Background(), &ypb.Empty{})
	require.NoError(t, err)
	require.True(t, second.Load().IsPassed("GET", "example.test", "/blocked", ""))
	require.EqualValues(t, 2, resets.Load())
	require.True(t, proto.Equal(second.Load().Data, GetMITMFilterManager(db, nil).Data))
	unregisterSecond()
	last := second.Load()
	require.NoError(t, first.Update(nil))
	require.Same(t, last, second.Load(), "closed sessions must not receive updates")
	require.EqualValues(t, 2, resets.Load())
	unregisterFirst()
	liveMITMFilters.Lock()
	_, registered := liveMITMFilters.byProject[db.DB()]
	liveMITMFilters.Unlock()
	require.False(t, registered, "the final session must release the project entry")
}

func TestLiveMITMFilterPersistenceFailure(t *testing.T) {
	db := newLiveMITMFilterTestDB(t)
	active, unregister := newActiveMITMFilter(db, nil)
	defer unregister()
	before := active.Load()
	require.NoError(t, db.Close())
	server := &Server{projectDatabase: db}
	_, err := server.SetMITMFilter(context.Background(), &ypb.SetMITMFilterRequest{FilterData: &ypb.MITMFilterData{}})
	require.Error(t, err)
	_, err = server.ResetMITMFilter(context.Background(), &ypb.Empty{})
	require.Error(t, err)
	require.Same(t, before, active.Load(), "failed persistence must not publish a new filter")
	require.Empty(t, active.changed)
}

func TestLiveMITMFilterProfileFallback(t *testing.T) {
	profile, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "profile.db"))
	require.NoError(t, err)
	require.NoError(t, profile.AutoMigrate(&schema.GeneralStorage{}).Error)
	t.Cleanup(func() { _ = profile.Close() })
	fallback := NewMITMFilter(&ypb.MITMFilterData{IncludeUri: []*ypb.FilterDataItem{{MatcherType: "word", Group: []string{"/fallback"}}}})
	fallback.db = profile
	require.NoError(t, fallback.SaveToDb())
	profileValue := yakit.GetKey(profile, yakit.MITMFilterKeyRecords)
	db := newLiveMITMFilterTestDB(t)
	active, unregister := newActiveMITMFilter(db, profile)
	defer unregister()
	require.False(t, active.Load().IsPassed("GET", "example.test", "/new", ""))
	require.NoError(t, active.Update(&ypb.MITMFilterData{}))
	require.True(t, active.Load().IsPassed("GET", "example.test", "/new", ""))
	require.True(t, proto.Equal(active.Load().Data, GetMITMFilterManager(db, profile).Data))
	require.Equal(t, profileValue, yakit.GetKey(profile, yakit.MITMFilterKeyRecords), "session edits must stay in the project")
}

func TestLiveMITMFilterConcurrentUpdates(t *testing.T) {
	db := newLiveMITMFilterTestDB(t)
	active, unregister := newActiveMITMFilter(db, nil)
	defer unregister()
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				data := &ypb.MITMFilterData{ExcludeUri: []*ypb.FilterDataItem{{MatcherType: "word", Group: []string{fmt.Sprintf("/%d/%d", worker, i)}}}}
				if _, err := saveMITMFilter(db, data, nil); err != nil {
					t.Error(err)
					return
				}
				joined, leave := newActiveMITMFilter(db, nil)
				joined.Load().IsPassed("GET", "example.test", "/traffic", "")
				leave()
			}
		}(worker)
	}
	for reader := 0; reader < 4; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				current := active.Load()
				current.IsPassed("GET", "example.test", "/traffic", "")
				current.IsMIMEPassed("text/plain")
				current.Data.GetFilterBundledStaticJS()
			}
		}()
	}
	wg.Wait()
	require.True(t, proto.Equal(active.Load().Data, GetMITMFilterManager(db, nil).Data), "last persisted update must also be the active one")
}
