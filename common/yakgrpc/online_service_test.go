package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// An unexpected remote call panics instead of silently contacting the network.
// Persistence is deliberately not part of this test double.
type stubOnlineService struct {
	uploadTemplate   func(context.Context, string, []byte) error
	downloadTemplate func(string, string, string) (*yaklib.HotPatchTemplate, error)
	uploadPayload    func(context.Context, string, []byte, []byte) error
	downloadPayload  func(context.Context, string, string, string) *yaklib.OnlineDownloadPayloadStream
	upload           func(context.Context, string, []byte, string) error
	uploadFlow       func(context.Context, *ypb.HTTPFlowsToOnlineRequest, []byte) error
	apiKey           func(context.Context, string) (string, error)
	downloadRules    func(context.Context, string, *ypb.DownloadSyntaxFlowRuleRequest) *yaklib.OnlineDownloadFlowRuleStream
}

func (s *stubOnlineService) UploadHotPatchTemplateToOnline(c context.Context, t string, b []byte) error {
	return s.uploadTemplate(c, t, b)
}
func (s *stubOnlineService) DownloadHotPatchTemplate(t, n, k string) (*yaklib.HotPatchTemplate, error) {
	return s.downloadTemplate(t, n, k)
}
func (s *stubOnlineService) UploadPayloadsToOnline(c context.Context, t string, b, f []byte) error {
	return s.uploadPayload(c, t, b, f)
}
func (s *stubOnlineService) DownloadBatchPayloads(c context.Context, t, g, f string) *yaklib.OnlineDownloadPayloadStream {
	return s.downloadPayload(c, t, g, f)
}
func (s *stubOnlineService) UploadToOnline(c context.Context, t string, b []byte, u string) error {
	return s.upload(c, t, b, u)
}
func (s *stubOnlineService) UploadHTTPFlowToOnline(c context.Context, r *ypb.HTTPFlowsToOnlineRequest, b []byte) error {
	return s.uploadFlow(c, r, b)
}
func (s *stubOnlineService) GetAIApiKeyByOnline(c context.Context, t string) (string, error) {
	return s.apiKey(c, t)
}
func (s *stubOnlineService) DownloadOnlineSyntaxFlowRule(c context.Context, t string, r *ypb.DownloadSyntaxFlowRuleRequest) *yaklib.OnlineDownloadFlowRuleStream {
	return s.downloadRules(c, t, r)
}

func newOnlineTestDB(t *testing.T, models ...interface{}) *gorm.DB {
	t.Helper()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(models...).Error)
	return db
}
