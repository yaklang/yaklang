package sfdb

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"gopkg.in/yaml.v3"
)

// PackageYAML is the on-disk / zip manifest (docs/design/rule-package.md).
type PackageYAML struct {
	Name        string              `yaml:"name" json:"name"`
	Version     string              `yaml:"version" json:"version"`
	Description string              `yaml:"description" json:"description"`
	Source      string              `yaml:"source" json:"source"`
	Rules       []PackageYAMLRule   `yaml:"rules" json:"rules"`
}

// PackageYAMLRule is one rule entry in package.yaml.
type PackageYAMLRule struct {
	RuleID   string `yaml:"rule_id" json:"rule_id"`
	RuleName string `yaml:"rule_name" json:"rule_name"`
	Version  string `yaml:"version" json:"version"`
	Hash     string `yaml:"hash,omitempty" json:"hash,omitempty"`
}

// PackageConflict describes a dual-key mismatch during import/sync.
type PackageConflict struct {
	RuleID         string
	RuleName       string
	LocalVersion   string
	RemoteVersion  string
	Reason         string
	PackageName    string
}

const (
	ConflictReasonIDNameMismatch = "id_name_mismatch"
	ConflictReasonNameCollision  = "name_collision"
	ConflictReasonIDCollision    = "id_collision"
)

// LoadPackageYAML reads package.yaml from a directory or file path.
func LoadPackageYAML(path string) (*PackageYAML, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	filePath := path
	if fi.IsDir() {
		filePath = filepath.Join(path, "package.yaml")
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, utils.Wrapf(err, "read package.yaml failed: %s", filePath)
	}
	return ParsePackageYAML(raw)
}

// ParsePackageYAML unmarshals package.yaml bytes.
func ParsePackageYAML(raw []byte) (*PackageYAML, error) {
	var meta PackageYAML
	if err := yaml.Unmarshal(raw, &meta); err != nil {
		return nil, utils.Wrap(err, "parse package.yaml failed")
	}
	meta.Name = strings.TrimSpace(meta.Name)
	meta.Version = strings.TrimSpace(meta.Version)
	if meta.Name == "" {
		return nil, utils.Error("package.yaml: name is required")
	}
	if meta.Version == "" {
		meta.Version = "0.1.0"
	}
	if meta.Source == "" {
		meta.Source = schema.SyntaxFlowPackageSourceLocal
	}
	return &meta, nil
}

// WritePackageYAML writes package.yaml into dir.
func WritePackageYAML(dir string, meta *PackageYAML) error {
	if meta == nil {
		return utils.Error("package meta is nil")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := yaml.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "package.yaml"), raw, 0o644)
}

// GetOrCreatePackage ensures a package row exists.
func GetOrCreatePackage(db *gorm.DB, name, version, description, source string, builtin bool) (*schema.SyntaxFlowPackage, error) {
	if db == nil {
		return nil, utils.Error("db is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, utils.Error("package name is empty")
	}
	var pkg schema.SyntaxFlowPackage
	err := db.Where("name = ?", name).First(&pkg).Error
	if err == nil {
		changed := false
		if version != "" && pkg.Version != version {
			pkg.Version = version
			changed = true
		}
		if description != "" && pkg.Description != description {
			pkg.Description = description
			changed = true
		}
		if source != "" && pkg.Source != source {
			pkg.Source = source
			changed = true
		}
		if pkg.IsBuiltin != builtin {
			pkg.IsBuiltin = builtin
			changed = true
		}
		if changed {
			if err := db.Save(&pkg).Error; err != nil {
				return nil, err
			}
		}
		return &pkg, nil
	}
	if !gorm.IsRecordNotFoundError(err) {
		return nil, err
	}
	if version == "" {
		version = "0.1.0"
	}
	if source == "" {
		if builtin {
			source = schema.SyntaxFlowPackageSourceEmbed
		} else {
			source = schema.SyntaxFlowPackageSourceUser
		}
	}
	pkg = schema.SyntaxFlowPackage{
		Name:        name,
		Version:     version,
		Description: description,
		Source:      source,
		IsBuiltin:   builtin,
	}
	if err := db.Create(&pkg).Error; err != nil {
		return nil, err
	}
	return &pkg, nil
}

// QueryPackageByName returns a package by name.
func QueryPackageByName(db *gorm.DB, name string) (*schema.SyntaxFlowPackage, error) {
	var pkg schema.SyntaxFlowPackage
	if err := db.Where("name = ?", name).First(&pkg).Error; err != nil {
		return nil, err
	}
	return &pkg, nil
}

// CountRulesInPackage counts rules belonging to a package.
func CountRulesInPackage(db *gorm.DB, packageName string) int64 {
	var count int64
	db.Model(&schema.SyntaxFlowRule{}).Where("package_name = ?", packageName).Count(&count)
	return count
}

// FilterSyntaxFlowPackages applies package filter.
func FilterSyntaxFlowPackages(db *gorm.DB, names, sources []string, keyword, builtinKind string) *gorm.DB {
	db = db.Model(&schema.SyntaxFlowPackage{})
	db = bizhelper.ExactOrQueryStringArrayOr(db, "name", names)
	db = bizhelper.ExactOrQueryStringArrayOr(db, "source", sources)
	if keyword != "" {
		db = bizhelper.FuzzSearch(db, []string{"name", "description"}, keyword)
	}
	switch strings.ToLower(builtinKind) {
	case "builtin", "buildin", "true":
		db = db.Where("is_builtin = ?", true)
	case "unbuiltin", "unbuildin", "false":
		db = db.Where("is_builtin = ?", false)
	}
	return db
}

// DeletePackage deletes a package row; optionally deletes its rules.
func DeletePackage(db *gorm.DB, name string, deleteRules bool) error {
	pkg, err := QueryPackageByName(db, name)
	if err != nil {
		return err
	}
	if pkg.IsBuiltin {
		return utils.Errorf("cannot delete builtin package: %s", name)
	}
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		if deleteRules {
			if err := tx.Where("package_name = ?", name).Unscoped().Delete(&schema.SyntaxFlowRule{}).Error; err != nil {
				return err
			}
		} else {
			// orphan rules → custom
			if err := tx.Model(&schema.SyntaxFlowRule{}).Where("package_name = ?", name).
				Update("package_name", schema.SyntaxFlowPackageCustom).Error; err != nil {
				return err
			}
		}
		return tx.Unscoped().Where("name = ?", name).Delete(&schema.SyntaxFlowPackage{}).Error
	})
}

// CheckRulePackageIdentityConflict checks dual-key uniqueness against DB for an incoming rule.
// Returns nil if safe to insert/update by matching both keys; otherwise a conflict.
func CheckRulePackageIdentityConflict(db *gorm.DB, packageName, ruleID, ruleName, remoteVersion string) *PackageConflict {
	if ruleID == "" && ruleName == "" {
		return nil
	}
	var byID schema.SyntaxFlowRule
	errID := db.Where("rule_id = ?", ruleID).First(&byID).Error
	var byName schema.SyntaxFlowRule
	errName := db.Where("rule_name = ?", ruleName).First(&byName).Error

	idFound := errID == nil
	nameFound := errName == nil

	if !idFound && !nameFound {
		return nil
	}
	if idFound && nameFound {
		if byID.ID == byName.ID {
			// same row: ok to version-update
			return nil
		}
		// two different rows — severe inconsistency
		return &PackageConflict{
			RuleID:        ruleID,
			RuleName:      ruleName,
			LocalVersion:  byID.Version,
			RemoteVersion: remoteVersion,
			Reason:        ConflictReasonIDNameMismatch,
			PackageName:   packageName,
		}
	}
	if idFound && byID.RuleName != ruleName {
		return &PackageConflict{
			RuleID:        ruleID,
			RuleName:      ruleName,
			LocalVersion:  byID.Version,
			RemoteVersion: remoteVersion,
			Reason:        ConflictReasonIDNameMismatch,
			PackageName:   packageName,
		}
	}
	if nameFound && byName.RuleId != ruleID {
		return &PackageConflict{
			RuleID:        ruleID,
			RuleName:      ruleName,
			LocalVersion:  byName.Version,
			RemoteVersion: remoteVersion,
			Reason:        ConflictReasonNameCollision,
			PackageName:   packageName,
		}
	}
	return nil
}

// PackageNeedsUpdate reports whether remote package semver is newer than installed.
func PackageNeedsUpdate(localVersion, remoteVersion string) bool {
	if remoteVersion == "" {
		return false
	}
	if localVersion == "" {
		return true
	}
	if localVersion == remoteVersion {
		return false
	}
	// CheckNewerVersion(base, check) == true means base > check (local newer).
	return !CheckNewerVersion(localVersion, remoteVersion)
}

// BuildPackageYAMLFromDB builds a package.yaml snapshot for export/CI.
func BuildPackageYAMLFromDB(db *gorm.DB, packageName string) (*PackageYAML, error) {
	pkg, err := QueryPackageByName(db, packageName)
	if err != nil {
		return nil, err
	}
	var rules []*schema.SyntaxFlowRule
	if err := db.Where("package_name = ?", packageName).Find(&rules).Error; err != nil {
		return nil, err
	}
	meta := &PackageYAML{
		Name:        pkg.Name,
		Version:     pkg.Version,
		Description: pkg.Description,
		Source:      pkg.Source,
	}
	for _, r := range rules {
		meta.Rules = append(meta.Rules, PackageYAMLRule{
			RuleID:   r.RuleId,
			RuleName: r.RuleName,
			Version:  r.Version,
			Hash:     r.Hash,
		})
	}
	return meta, nil
}

// EnsureCustomPackage creates the default user package if missing.
func EnsureCustomPackage(db *gorm.DB) (*schema.SyntaxFlowPackage, error) {
	return GetOrCreatePackage(db, schema.SyntaxFlowPackageCustom, "0.1.0", "User local rules", schema.SyntaxFlowPackageSourceUser, false)
}

// LogPackageConflict helper for CLI/grpc.
func LogPackageConflict(c *PackageConflict) {
	if c == nil {
		return
	}
	log.Warnf("syntaxflow package conflict: pkg=%s rule_id=%s rule_name=%s reason=%s local=%s remote=%s",
		c.PackageName, c.RuleID, c.RuleName, c.Reason, c.LocalVersion, c.RemoteVersion)
}
