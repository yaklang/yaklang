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
	"github.com/yaklang/yaklang/common/syntaxflow/sfrisk"
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
		RiskType: "sqli",
		AlertDesc: schema.MapEx[string, *schema.SyntaxFlowDescInfo]{
			"result": {RiskType: "SQL注入", TitleZh: "SQL 注入"},
		},
	}
	require.NoError(t, db.Create(rule).Error)
	standardGroups := append(ssaconfig.GetAllSupportedLanguages(), ssaconfig.General.String(), "go", "javascript")
	rawGroups := append(append([]string(nil), standardGroups...), "user-defined-group")
	for _, name := range rawGroups {
		group := &schema.SyntaxFlowGroup{GroupName: name}
		require.NoError(t, db.Create(group).Error)
		require.NoError(t, db.Model(rule).Association("Groups").Append(group).Error)
	}

	archive, result, err := ExportRulesToBytes(context.Background(), db)
	require.NoError(t, err)
	require.Equal(t, 1, result.Count)

	metadata := readRuleExportMetadata(t, archive)
	require.Equal(t, sfrisk.GetTaxonomy(), metadata.RiskTypeTaxonomy)
	require.Equal(t, ssaconfig.RuleGroupTaxonomySchemaVersion, metadata.RuleGroupTaxonomy.SchemaVersion)
	require.NotEmpty(t, metadata.RuleGroupTaxonomy.Categories)
	taxonomyNames := make(map[string]bool)
	for _, category := range metadata.RuleGroupTaxonomy.Categories {
		for _, group := range category.Groups {
			taxonomyNames[group.Name] = true
		}
	}
	for _, name := range standardGroups {
		require.True(t, taxonomyNames[name], "exported raw group %q must have exact taxonomy metadata", name)
	}
	require.False(t, taxonomyNames["user-defined-group"], "custom groups must not become standard taxonomy")
	require.Equal(t, "taxonomy-rule-id", metadata.Relationship[0].RuleID)
	require.ElementsMatch(t, rawGroups, metadata.Relationship[0].GroupNames)

	importDB, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, importDB.AutoMigrate(&schema.SyntaxFlowGroup{}, &schema.SyntaxFlowRule{}).Error)
	importedMetadata, err := ImportRulesFromBytes(context.Background(), importDB, archive)
	require.NoError(t, err)
	require.Contains(t, importedMetadata, "rule_group_taxonomy")
	require.Contains(t, importedMetadata, "risk_type_taxonomy")

	var imported schema.SyntaxFlowRule
	require.NoError(t, importDB.Preload("Groups").Where("rule_id = ?", rule.RuleId).First(&imported).Error)
	require.Equal(t, rule.RuleId, imported.RuleId)
	require.Equal(t, rule.Content, imported.Content)
	require.Equal(t, rule.Hash, imported.Hash)
	require.Equal(t, "sqli", imported.RiskType)
	require.Equal(t, rule.AlertDesc, imported.AlertDesc)
	var importedGroupNames []string
	for _, group := range imported.Groups {
		importedGroupNames = append(importedGroupNames, group.GroupName)
	}
	require.ElementsMatch(t, rawGroups, importedGroupNames)
}

type ruleExportMetadata struct {
	RuleGroupTaxonomy ssaconfig.RuleGroupTaxonomy `json:"rule_group_taxonomy"`
	RiskTypeTaxonomy  sfrisk.Taxonomy             `json:"risk_type_taxonomy"`
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
