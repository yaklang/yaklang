package yakgrpc

import (
	"errors"
	"fmt"

	"github.com/mattn/go-sqlite3"
	"github.com/samber/lo"
	"github.com/tidwall/sjson"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) ExportFingerprint(req *ypb.ExportFingerprintRequest, stream ypb.Yak_ExportFingerprintServer) error {
	db := consts.GetGormProfileDatabase()
	ruleDB := yakit.FilterGeneralRule(db.Model(&schema.GeneralRule{}).Preload("Groups"), req.GetFilter())
	var rules []*schema.GeneralRule
	if err := ruleDB.Find(&rules).Error; err != nil {
		return err
	}
	totalGroupNames := make([]string, 0, len(rules))
	metadata := make(bizhelper.MetaData)
	metadata["relationship"] = lo.Map(rules, func(item *schema.GeneralRule, index int) map[string]any {
		groupNames := lo.Map(item.Groups, func(item *schema.GeneralRuleGroup, index int) string {
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
		return utils.Wrap(ruleDB.Error, "get fingerprint rule count failed")
	}
	if ruleCount == 0 {
		return utils.Error("no fingerprint rule found")
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
		stream.Send(&ypb.DataTransferProgress{
			Progress: progress,
		})
	}))
	opts = append(opts, bizhelper.WithExportPreWriteHandler(func(name string, w []byte, metadata bizhelper.MetaData) (newName string, new []byte) {
		nw, err := sjson.DeleteBytes(w, "CreatedAt")
		if err != nil {
			return name, w
		}
		w = nw

		nw, err = sjson.DeleteBytes(w, "UpdatedAt")
		if err != nil {
			return name, w
		}
		w = nw

		return name, w
	}))
	opts = append(opts, bizhelper.WithExportIndexField(`"general_rules".id`))
	err := bizhelper.ExportTableZip[*schema.GeneralRule](stream.Context(), ruleDB, req.GetTargetPath(), opts...)
	if err != nil {
		return utils.Wrap(err, "export fingerprint rules failed")
	}

	return nil
}

func (s *Server) ImportFingerprint(req *ypb.ImportFingerprintRequest, stream ypb.Yak_ImportFingerprintServer) error {
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
		stream.Send(&ypb.DataTransferProgress{
			Progress: progress,
		})
	}))

	opts = append(opts, bizhelper.WithImportErrorHandler(func(err error) (newErr error) {
		var sqlErr sqlite3.Error
		if errors.As(err, &sqlErr) && sqlErr.Code == sqlite3.ErrConstraint {
			// ignore duplicate error, just send message
			err = nil
			stream.Send(&ypb.DataTransferProgress{
				Verbose: fmt.Sprintf("duplicate rule, skip: %s", sqlErr.Error()),
			})
		}
		return err
	}))

	ruleDB := db.Model(&schema.GeneralRule{})
	err := bizhelper.ImportTableZip[*schema.GeneralRule](stream.Context(), ruleDB, req.GetInputPath(), opts...)
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

			rule, err := yakit.GetGeneralRuleByRuleName(db, ruleName)
			if err != nil {
				return utils.Wrapf(err, "not found rule: %s", ruleName)
			}
			_, err = yakit.BatchAppendGeneralRuleGroupAssociations(db, []*schema.GeneralRule{rule}, groupNames)
			if err != nil {
				return utils.Wrap(err, "batch add groups for rules failed")
			}
		}
	}
	return nil
}
