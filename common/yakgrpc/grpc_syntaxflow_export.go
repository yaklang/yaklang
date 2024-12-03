package yakgrpc

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) ExportSyntaxFlows(req *ypb.ExportSyntaxFlowsRequest, stream ypb.Yak_ExportSyntaxFlowsServer) error {
	db := consts.GetGormProfileDatabase()
	groupDB := db.Model(&schema.SyntaxFlowGroup{}).Preload("Rules")
	groupDB = bizhelper.ExactQueryStringArrayOr(groupDB, "group_name", req.GetGroupName())
	var groups []*schema.SyntaxFlowGroup
	if groupDB := groupDB.Find(&groups); groupDB.Error != nil {
		return utils.Wrap(groupDB.Error, "get syntax flow rule group failed")
	}
	totalRuleNames := make([]string, 0, len(groups))

	metadata := make(bizhelper.MetaData)
	metadata["group"] = lo.Map(groups, func(item *schema.SyntaxFlowGroup, index int) map[string]any {
		ruleNames := lo.Map(item.Rules, func(item *schema.SyntaxFlowRule, index int) string {
			return item.RuleName
		})
		totalRuleNames = append(totalRuleNames, ruleNames...)
		return map[string]any{
			"group_name": item.GroupName,
			"rule_names": ruleNames,
		}
	})

	ruleDB := bizhelper.ExactQueryStringArrayOr(db.Model(&schema.SyntaxFlowRule{}), "rule_name", totalRuleNames)
	ruleCount, handled := 0, 0
	progress := 0.0
	if ruleDB := ruleDB.Count(&ruleCount); ruleDB.Error != nil {
		return utils.Wrap(ruleDB.Error, "get syntax flow rule count failed")
	}
	metadata["count"] = ruleCount

	opts := make([]bizhelper.ExportOption, 0)
	if req.GetPassword() != "" {
		opts = append(opts, bizhelper.WithExportPassword(req.GetPassword()))
	}

	opts = append(opts, bizhelper.WithExportMetadata(metadata))
	opts = append(opts, bizhelper.WithExportAfterWriteHandler(func(name string, w []byte, metadata map[string]any) {
		handled++
		progress = float64(handled) / float64(ruleCount)
		stream.Send(&ypb.SyntaxflowsProgress{
			Progress: progress,
		})
	}))
	err := bizhelper.ExportTableZip[*schema.SyntaxFlowRule](stream.Context(), ruleDB, req.GetTargetPath(), opts...)
	if err != nil {
		return utils.Wrap(err, "export syntax flow rules failed")
	}

	return nil
}

func (s *Server) ImportSyntaxFlows(req *ypb.ImportSyntaxFlowsRequest, stream ypb.Yak_ImportSyntaxFlowsServer) error {
	ruleCount, handled := 0, 0
	progress := 0.0
	db := s.GetProfileDatabase()

	opts := make([]bizhelper.ImportOption, 0)
	if req.GetPassword() != "" {
		opts = append(opts, bizhelper.WithImportPassword(req.GetPassword()))
	}
	var metadata bizhelper.MetaData

	opts = append(opts, bizhelper.WithMetaDataHandler(func(m bizhelper.MetaData) error {
		metadata = m
		ruleCount = utils.InterfaceToInt(metadata["count"])
		if ruleCount == 0 {
			return utils.Error("metadata: invalid rule count")
		}
		return nil
	}))

	opts = append(opts, bizhelper.WithImportAfterReadHandler(func(name string, b []byte, metadata bizhelper.MetaData) {
		handled++
		progress = float64(handled) / float64(ruleCount)
		stream.Send(&ypb.SyntaxflowsProgress{
			Progress: progress,
		})
	}))

	ruleDB := db.Model(&schema.SyntaxFlowRule{})
	err := bizhelper.ImportTableZip[*schema.SyntaxFlowRule](stream.Context(), ruleDB, req.GetInputPath(), opts...)
	if err != nil {
		return err
	}

	// recover groups
	iGroups, ok := metadata["group"]
	if !ok {
		return utils.Error("metadata: invalid metadata")
	}
	m, ok := iGroups.([]any)
	if !ok {
		return utils.Error("metadata: invalid metadata type")
	}
	for _, iItem := range m {
		item, ok := iItem.(map[string]any)
		if !ok {
			return utils.Error("metadata: invalid metadata item")
		}
		groupName, ok := item["group_name"].(string)
		if !ok {
			return utils.Error("metadata: group_name invalid")
		}
		iRuleNames, ok := item["rule_names"].([]any)
		if !ok {
			return utils.Error("metadata: rule_names invalid")
		}
		if len(iRuleNames) > 0 {
			ruleNames := lo.Map(iRuleNames, func(item any, index int) string { return utils.InterfaceToString(item) })
			_, err := sfdb.BatchAddGroupsForRules(db, ruleNames, []string{groupName})
			if err != nil {
				return utils.Wrap(err, "batch add groups for rules failed")
			}
		}
	}
	return nil
}
