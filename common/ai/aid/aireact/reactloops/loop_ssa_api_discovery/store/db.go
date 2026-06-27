package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/utils"
)

const subDir = "ssa_discovery"
const dbFileName = "session.sqlite3"

// SubDirName returns the directory name under workdir (ssa_discovery).
func SubDirName() string { return subDir }

// OpenSessionDB opens (and migrates) the per-task SQLite under workDir/ssa_discovery/session.sqlite3.
func OpenSessionDB(workDir string) (*gorm.DB, error) {
	if workDir == "" {
		return nil, utils.Error("workDir is empty")
	}
	dir := filepath.Join(workDir, subDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, utils.Wrapf(err, "mkdir discovery dir")
	}
	pure := filepath.Join(dir, dbFileName)
	dsn := fmt.Sprintf("%s?cache=shared&mode=rwc", pure)
	db, err := gorm.Open("sqlite3", dsn)
	if err != nil {
		return nil, utils.Wrapf(err, "open sqlite %s", pure)
	}
	sqlDB := db.DB()
	if sqlDB != nil {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	}
	if err := AutoMigrate(db); err != nil {
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		return nil, err
	}
	if err := migrateSQLiteVulnVerificationTextColumns(db); err != nil {
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		return nil, utils.Wrap(err, "migrate vuln_verifications text columns")
	}
	return db, nil
}

// migrateSQLiteVulnVerificationTextColumns rebuilds vuln_verifications when legacy VARCHAR(255)
// definitions would truncate evidence blobs in DDL-oriented tooling / future drivers.
func migrateSQLiteVulnVerificationTextColumns(db *gorm.DB) error {
	if db == nil || db.Dialect() == nil || db.Dialect().GetName() != "sqlite3" {
		return nil
	}
	if !db.HasTable(&VulnVerification{}) {
		return nil
	}

	needsRebuild := false
	rows, err := db.Raw("PRAGMA table_info(`vuln_verifications`)").Rows()
	if err != nil {
		return utils.Wrap(err, "pragma vuln_verifications")
	}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt interface{}
		if rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk) != nil {
			continue
		}
		name = strings.TrimSpace(name)
		ctypeLow := strings.ToLower(strings.TrimSpace(ctype))
		switch name {
		case "exploit_payload", "exploit_response", "ai_analysis", "fix":
			if strings.Contains(ctypeLow, "varchar") {
				needsRebuild = true
			}
		default:
		}
		if needsRebuild {
			break
		}
	}
	pErr := rows.Err()
	closeErr := rows.Close()
	if pErr != nil {
		return utils.Wrap(pErr, "iterate pragma vuln_verifications")
	}
	if closeErr != nil {
		return utils.Wrap(closeErr, "close pragma rows")
	}

	if needsRebuild {
		tx := db.Begin()
		if tx.Error != nil {
			return utils.Wrap(tx.Error, "migrate tx begin")
		}
		if err := tx.Exec("ALTER TABLE `vuln_verifications` RENAME TO `vuln_verifications__old__mig`").Error; err != nil {
			tx.Rollback()
			return utils.Wrap(err, "rename vuln_verifications for migration")
		}
		if err := AutoMigrate(tx); err != nil {
			tx.Rollback()
			return utils.Wrap(err, "automigrate after rename")
		}
		insertSQL := `INSERT INTO vuln_verifications (id, created_at, updated_at, session_id, syntax_flow_finding_id, status, confidence, exploit_payload, exploit_response, ai_analysis, fix)
SELECT id, created_at, updated_at, session_id, syntax_flow_finding_id, status, confidence, exploit_payload, exploit_response, ai_analysis, fix
FROM vuln_verifications__old__mig`
		if err := tx.Exec(insertSQL).Error; err != nil {
			tx.Rollback()
			return utils.Wrap(err, "copy vuln_verifications migration")
		}
		if err := tx.Exec("DROP TABLE vuln_verifications__old__mig").Error; err != nil {
			tx.Rollback()
			return utils.Wrap(err, "drop old vuln_verifications")
		}
		if err := tx.Commit().Error; err != nil {
			tx.Rollback()
			return utils.Wrap(err, "migrate commit")
		}
	}
	return nil
}

// AutoMigrate creates tables for the discovery store.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&DiscoverySession{},
		&ArchitectureComponent{},
		&ConfigArtifact{},
		&DependencyRef{},
		&HttpEndpoint{},
		&SecurityMechanism{},
		&BusinessCapability{},
		&DiscoveryEvent{},
		&VerifiedEndpoint{},
		&DiscoverySyntaxFlowFinding{},
		&VulnVerification{},
		&AuthCredential{},
		&AuthAcquisitionRecipe{},
		&EndpointValidationAttempt{},
		&DynamicVulnFinding{},
		&CoverageWorkItem{},
		&VerifiedHttpApi{},
		&VulnChecklistItem{},
		&PhaseArtifact{},
		&DiscoveryFileOperation{},
		&EndpointVulnProbe{},
	).Error
}

// DBPath returns the filesystem path of the session DB file for logging / UI.
func DBPath(workDir string) string {
	return filepath.Join(workDir, subDir, dbFileName)
}
