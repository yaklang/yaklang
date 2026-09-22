package yakgrpc

import (
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func NewLocalClientAndServerWithTempDatabase(t *testing.T) (ypb.YakClient, *Server, error) {
	t.Helper()
	dir := t.TempDir()
	s, err := newServerEx(WithProfileDatabasePath(filepath.Join(dir, "profile.db")), WithProjectDatabasePath(filepath.Join(dir, "project.db")))
	if err != nil {
		return nil, nil, err
	}
	t.Cleanup(func() {
		if s.projectReadDatabase != nil {
			_ = s.projectReadDatabase.Close()
		}
		_ = s.projectDatabase.Close()
		_ = s.profileDatabase.Close()
	})
	client, err := newInMemoryClient(s)
	if err != nil {
		return nil, nil, err
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, s, nil
}

func NewLocalClientWithTempDatabase(t *testing.T) (ypb.YakClient, error) {
	t.Helper()
	client, _, err := NewLocalClientAndServerWithTempDatabase(t)
	return client, err
}
