package yakgrpc

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) ExportSyntaxFlows(req *ypb.ExportSyntaxFlowsRequest, stream ypb.Yak_ExportSyntaxFlowsServer) error {
	db := consts.GetGormProfileDatabase()
	ruleDB := yakit.FilterSyntaxFlowRule(db, req.GetFilter())
	ruleGroupDB := ruleDB.Select(`"syntax_flow_rules".id, "syntax_flow_rules".rule_name`).Preload("Groups")
	var rules []*schema.SyntaxFlowRule
	if ruleDB := ruleGroupDB.Find(&rules); ruleDB.Error != nil {
		return utils.Wrap(ruleDB.Error, "get syntax flow group failed")
	}
	totalGroupNames := make([]string, 0, len(rules))
	metadata := make(bizhelper.MetaData)
	metadata["relationship"] = lo.Map(rules, func(item *schema.SyntaxFlowRule, index int) map[string]any {
		groupNames := lo.Map(item.Groups, func(item *schema.SyntaxFlowGroup, index int) string {
			return item.GroupName
		})
		totalGroupNames = append(totalGroupNames, groupNames...)
		return map[string]any{
			"rule_name":   item.RuleName,
			"group_names": groupNames,
		}
	})

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
	iGroups, ok := metadata["relationship"]
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
		ruleName, ok := item["rule_name"].(string)
		if !ok {
			return utils.Error("metadata: rule_name invalid")
		}
		iGroupNames, ok := item["group_names"].([]any)
		if !ok {
			return utils.Error("metadata: group_names invalid")
		}
		if len(iGroupNames) > 0 {
			groupNames := lo.Map(iGroupNames, func(item any, index int) string { return utils.InterfaceToString(item) })

			_, err := sfdb.BatchAddGroupsForRules(db, []string{ruleName}, groupNames)
			if err != nil {
				return utils.Wrap(err, "batch add groups for rules failed")
			}
		}
	}
	return nil
}
