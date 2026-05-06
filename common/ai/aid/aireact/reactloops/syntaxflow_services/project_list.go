package syntaxflow_services

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ListProjectsText formats a short SSA project listing for AI actions / timeline.
func ListProjectsText(search, language string, limit int) (string, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return "", utils.Error("profile database not available")
	}
	if limit <= 0 {
		limit = 30
	}
	f := &ypb.SSAProjectFilter{}
	if strings.TrimSpace(search) != "" {
		f.SearchKeyword = strings.TrimSpace(search)
	}
	if lang := strings.TrimSpace(language); lang != "" {
		f.Languages = []string{lang}
	}
	_, projects, err := yakit.QuerySSAProject(db, &ypb.QuerySSAProjectRequest{
		Filter: f,
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   int64(limit),
			OrderBy: "updated_at",
			Order:   "desc",
		},
	})
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("SSA projects (showing %d):\n\n", len(projects)))
	for i, p := range projects {
		sb.WriteString(fmt.Sprintf("%d. id=%d name=%s lang=%s desc=%s\n",
			i+1, p.ID, utils.ShrinkTextBlock(p.ProjectName, 80), p.Language, utils.ShrinkTextBlock(p.Description, 60)))
	}
	return sb.String(), nil
}
