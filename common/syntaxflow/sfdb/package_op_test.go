package sfdb

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

func TestParsePackageYAML(t *testing.T) {
	raw := []byte(`
name: builtin
version: "1.2.3"
description: test
source: embed
rules:
  - rule_id: "abc"
    rule_name: "r1"
    version: "20260101.0001"
`)
	meta, err := ParsePackageYAML(raw)
	require.NoError(t, err)
	require.Equal(t, "builtin", meta.Name)
	require.Equal(t, "1.2.3", meta.Version)
	require.Len(t, meta.Rules, 1)
	require.Equal(t, "abc", meta.Rules[0].RuleID)
}

func TestPackageNeedsUpdate(t *testing.T) {
	require.True(t, PackageNeedsUpdate("", "1.0.0"))
	require.False(t, PackageNeedsUpdate("1.0.0", "1.0.0"))
	require.True(t, PackageNeedsUpdate("1.0.0", "1.0.1"))
	require.False(t, PackageNeedsUpdate("1.0.1", "1.0.0"))
}

func TestGetOrCreatePackageAndConflict(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	require.NoError(t, db.AutoMigrate(&schema.SyntaxFlowPackage{}, &schema.SyntaxFlowRule{}).Error)

	pkgName := "pkg-" + uuid.NewString()
	pkg, err := GetOrCreatePackage(db, pkgName, "0.1.0", "test", schema.SyntaxFlowPackageSourceUser, false)
	require.NoError(t, err)
	require.Equal(t, pkgName, pkg.Name)
	t.Cleanup(func() {
		_ = DeletePackage(db, pkgName, true)
	})

	ruleID := uuid.NewString()
	ruleName := "rule-" + uuid.NewString()
	rule := &schema.SyntaxFlowRule{
		RuleId:      ruleID,
		RuleName:    ruleName,
		Content:     "desc(title: x)\na",
		PackageName: pkgName,
		Version:     "1",
	}
	require.NoError(t, db.Create(rule).Error)

	require.Nil(t, CheckRulePackageIdentityConflict(db, pkgName, ruleID, ruleName, "2"))
	c := CheckRulePackageIdentityConflict(db, pkgName, ruleID, "other-name", "2")
	require.NotNil(t, c)
	require.Equal(t, ConflictReasonIDNameMismatch, c.Reason)

	c2 := CheckRulePackageIdentityConflict(db, pkgName, uuid.NewString(), ruleName, "2")
	require.NotNil(t, c2)
	require.Equal(t, ConflictReasonNameCollision, c2.Reason)
}
