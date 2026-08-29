package sfdb

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestRuleExportEmbedsTaxonomyAndRemainsImportCompatible(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.SyntaxFlowGroup{}, &schema.SyntaxFlowRule{}).Error)

	rule := &schema.SyntaxFlowRule{
		RuleId:   "taxonomy-rule-id",
		RuleName: "taxonomy-rule",
		Content:  `desc(title: "taxonomy") alert $result`,
	}
	require.NoError(t, db.Create(rule).Error)
	group := &schema.SyntaxFlowGroup{GroupName: "java"}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Model(rule).Association("Groups").Append(group).Error)

	archive, result, err := ExportRulesToBytes(context.Background(), db)
	require.NoError(t, err)
	require.Equal(t, 1, result.Count)

	metadata := readRuleExportMetadata(t, archive)
	require.Equal(t, ssaconfig.RuleGroupTaxonomySchemaVersion, metadata.RuleGroupTaxonomy.SchemaVersion)
	require.NotEmpty(t, metadata.RuleGroupTaxonomy.Categories)
	taxonomyGroupCount := 0
	for _, category := range metadata.RuleGroupTaxonomy.Categories {
		taxonomyGroupCount += len(category.Groups)
	}
	require.Equal(t, 36, taxonomyGroupCount)
	require.Equal(t, "taxonomy-rule-id", metadata.Relationship[0].RuleID)
	require.Equal(t, []string{"java"}, metadata.Relationship[0].GroupNames)

	importDB, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, importDB.AutoMigrate(&schema.SyntaxFlowGroup{}, &schema.SyntaxFlowRule{}).Error)
	importedMetadata, err := ImportRulesFromBytes(context.Background(), importDB, archive)
	require.NoError(t, err)
	require.Contains(t, importedMetadata, "rule_group_taxonomy")

	var imported schema.SyntaxFlowRule
	require.NoError(t, importDB.Preload("Groups").Where("rule_id = ?", rule.RuleId).First(&imported).Error)
	require.Len(t, imported.Groups, 1)
	require.Equal(t, "java", imported.Groups[0].GroupName)
}

type ruleExportMetadata struct {
	RuleGroupTaxonomy ssaconfig.RuleGroupTaxonomy `json:"rule_group_taxonomy"`
	Relationship      []struct {
		RuleID     string   `json:"rule_id"`
		GroupNames []string `json:"group_names"`
	} `json:"relationship"`
}

func readRuleExportMetadata(t *testing.T, data []byte) ruleExportMetadata {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	for _, file := range reader.File {
		if filepath.Base(file.Name) != "meta.json" {
			continue
		}
		stream, err := file.Open()
		require.NoError(t, err)
		payload, err := io.ReadAll(stream)
		require.NoError(t, err)
		require.NoError(t, stream.Close())
		var metadata ruleExportMetadata
		require.NoError(t, json.Unmarshal(payload, &metadata))
		return metadata
	}
	t.Fatal("meta.json not found")
	return ruleExportMetadata{}
}
