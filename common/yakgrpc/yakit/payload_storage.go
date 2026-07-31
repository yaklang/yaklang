package yakit

import (
	"database/sql"
	"strconv"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const legacyPayloadFileFlagRepairKey = "__yaklang_internal_payload_file_flag_repair_v1__"

// PayloadGroupStorageMode describes how a payload group stores its content.
type PayloadGroupStorageMode uint8

const (
	PayloadGroupStorageEmpty PayloadGroupStorageMode = iota
	PayloadGroupStorageDatabase
	PayloadGroupStorageFile
	PayloadGroupStorageLegacyFileFlag
	PayloadGroupStorageInconsistent
)

func (m PayloadGroupStorageMode) String() string {
	switch m {
	case PayloadGroupStorageEmpty:
		return "empty"
	case PayloadGroupStorageDatabase:
		return "database"
	case PayloadGroupStorageFile:
		return "file"
	case PayloadGroupStorageLegacyFileFlag:
		return "legacy-misclassified-file"
	case PayloadGroupStorageInconsistent:
		return "inconsistent"
	default:
		return "unknown"
	}
}

func isQuotedDatabasePayload(content string) bool {
	if len(content) < 2 || content[0] != '"' || content[len(content)-1] != '"' {
		return false
	}
	_, err := strconv.Unquote(content)
	return err == nil
}

// InspectPayloadGroupStorage classifies a payload group using the storage
// invariant defined by schema.Payload:
//
//   - database groups contain one row per quoted payload and IsFile=false;
//   - file groups contain exactly one row whose raw Content is a file path and
//     IsFile=true.
//
// Older SavePayloadByFilename versions stored every quoted payload line with
// IsFile=true. Those groups are returned as PayloadGroupStorageLegacyFileFlag.
func InspectPayloadGroupStorage(db *gorm.DB, group string) (PayloadGroupStorageMode, error) {
	if db == nil {
		return PayloadGroupStorageEmpty, utils.Error("payload database is nil")
	}

	var total, fileCount int64
	if result := db.Model(&schema.Payload{}).
		Where("`group` = ?", group).
		Count(&total); result.Error != nil {
		return PayloadGroupStorageEmpty, utils.Wrap(result.Error, "count payload group records")
	}
	if total == 0 {
		return PayloadGroupStorageEmpty, nil
	}
	if result := db.Model(&schema.Payload{}).
		Where("`group` = ?", group).
		Where("is_file = ?", true).
		Count(&fileCount); result.Error != nil {
		return PayloadGroupStorageEmpty, utils.Wrap(result.Error, "count file payload group records")
	}
	if fileCount == 0 {
		return PayloadGroupStorageDatabase, nil
	}
	if fileCount != total {
		return PayloadGroupStorageInconsistent, nil
	}

	rows, err := db.Model(&schema.Payload{}).
		Select("content").
		Where("`group` = ?", group).
		Where("is_file = ?", true).
		Rows()
	if err != nil {
		return PayloadGroupStorageEmpty, utils.Wrap(err, "inspect file payload group content")
	}
	defer rows.Close()

	var rawFileCount, quotedFileCount int64
	for rows.Next() {
		var content sql.NullString
		if err := rows.Scan(&content); err != nil {
			return PayloadGroupStorageEmpty, utils.Wrap(err, "scan file payload group content")
		}
		if !content.Valid {
			continue
		}
		if isQuotedDatabasePayload(content.String) {
			quotedFileCount++
		} else {
			rawFileCount++
		}
	}
	if err := rows.Err(); err != nil {
		return PayloadGroupStorageEmpty, utils.Wrap(err, "iterate file payload group content")
	}

	switch {
	case quotedFileCount == fileCount:
		return PayloadGroupStorageLegacyFileFlag, nil
	case rawFileCount == 1 && fileCount == 1:
		return PayloadGroupStorageFile, nil
	default:
		return PayloadGroupStorageInconsistent, nil
	}
}

func repairLegacyFilePayloadGroup(db *gorm.DB, group string) (bool, error) {
	mode, err := InspectPayloadGroupStorage(db, group)
	if err != nil {
		return false, err
	}
	if mode != PayloadGroupStorageLegacyFileFlag {
		return false, nil
	}

	if err := db.Model(&schema.Payload{}).
		Where("`group` = ?", group).
		Update("is_file", false).Error; err != nil {
		return false, utils.Wrapf(err, "repair legacy payload group %q", group)
	}
	return true, nil
}

func repairLegacyFilePayloadGroups(db *gorm.DB) (int, error) {
	if db == nil {
		return 0, utils.Error("payload database is nil")
	}

	var groups []string
	if err := db.Model(&schema.Payload{}).
		Where("is_file = ?", true).
		Pluck("DISTINCT(`group`)", &groups).Error; err != nil {
		return 0, utils.Wrap(err, "query file payload groups")
	}

	repaired := 0
	for _, group := range groups {
		ok, err := repairLegacyFilePayloadGroup(db, group)
		if err != nil {
			return repaired, err
		}
		if ok {
			repaired++
		}
	}
	return repaired, nil
}

func repairLegacyPayloadFileFlagsPatch(db *gorm.DB) {
	if db == nil || !db.HasTable(&schema.Payload{}) || !db.HasTable(&schema.GeneralStorage{}) {
		return
	}
	if GetKey(db, legacyPayloadFileFlagRepairKey) == "done" {
		return
	}

	repaired, err := repairLegacyFilePayloadGroups(db)
	if err != nil {
		log.Warnf("failed to repair legacy payload file flags: %v", err)
		return
	}
	if err := SetKey(db, legacyPayloadFileFlagRepairKey, "done"); err != nil {
		log.Warnf("failed to record legacy payload file flag repair: %v", err)
		return
	}
	if repaired > 0 {
		log.Infof("repaired %d legacy payload group(s) incorrectly marked as file-backed", repaired)
	}
}

func init() {
	schema.RegisterDatabasePatch(schema.KEY_SCHEMA_PROFILE_DATABASE, repairLegacyPayloadFileFlagsPatch)
}
