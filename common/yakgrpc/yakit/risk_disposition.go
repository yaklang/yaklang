package yakit

import (
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	RiskDispositionBatchLimit = 200
	RiskDispositionReasonMax  = 4096
)

type RiskDispositionUpdateResult struct {
	UpdatedCount   int
	UnchangedCount int
	MissingIDs     []int64
}

// UpdateRiskDispositions changes only the recoverable disposition fields of
// existing risks. It never deletes a risk record.
func UpdateRiskDispositions(
	db *gorm.DB,
	ids []int64,
	disposition string,
	duplicateOf int64,
	reason string,
) (*RiskDispositionUpdateResult, error) {
	if db == nil {
		return nil, utils.Error("project database is unavailable")
	}

	disposition = schema.NormalizeRiskDisposition(disposition)
	if !schema.IsValidRiskDisposition(disposition) {
		return nil, fmt.Errorf("invalid risk disposition: %q", disposition)
	}

	ids, err := normalizeRiskDispositionIDs(ids)
	if err != nil {
		return nil, err
	}
	if len(ids) > RiskDispositionBatchLimit {
		return nil, fmt.Errorf("risk disposition batch exceeds %d IDs", RiskDispositionBatchLimit)
	}

	reason = strings.TrimSpace(reason)
	if len(reason) > RiskDispositionReasonMax {
		return nil, fmt.Errorf("risk disposition reason exceeds %d bytes", RiskDispositionReasonMax)
	}
	if disposition != schema.RiskDispositionActive && reason == "" {
		return nil, utils.Error("risk disposition reason is required except for restore")
	}

	if disposition == schema.RiskDispositionDuplicate {
		if duplicateOf <= 0 {
			return nil, utils.Error("duplicate_of must identify the canonical risk")
		}
		for _, id := range ids {
			if id == duplicateOf {
				return nil, utils.Error("duplicate_of cannot be one of the risks being marked as duplicate")
			}
		}
	} else {
		duplicateOf = 0
		if disposition == schema.RiskDispositionActive {
			reason = ""
		}
	}

	result := &RiskDispositionUpdateResult{}
	var updatedRisks []*schema.Risk
	err = db.Transaction(func(tx *gorm.DB) error {
		var risks []*schema.Risk
		if err := tx.Model(&schema.Risk{}).Where("id IN (?)", ids).Find(&risks).Error; err != nil {
			return utils.Wrap(err, "load risks for disposition")
		}

		found := make(map[int64]struct{}, len(risks))
		for _, risk := range risks {
			found[int64(risk.ID)] = struct{}{}
		}
		for _, id := range ids {
			if _, ok := found[id]; !ok {
				result.MissingIDs = append(result.MissingIDs, id)
			}
		}

		if disposition == schema.RiskDispositionDuplicate {
			var canonical schema.Risk
			if err := tx.Model(&schema.Risk{}).Where("id = ?", duplicateOf).First(&canonical).Error; err != nil {
				return fmt.Errorf("canonical risk %d is unavailable: %w", duplicateOf, err)
			}
			if schema.NormalizeRiskDisposition(canonical.Disposition) != schema.RiskDispositionActive {
				return fmt.Errorf("canonical risk %d must be active", duplicateOf)
			}
		}

		var changedIDs []uint
		for _, risk := range risks {
			currentDisposition := schema.NormalizeRiskDisposition(risk.Disposition)
			currentDuplicateOf := int64(risk.DuplicateOfRiskID)
			currentReason := strings.TrimSpace(risk.TagReason)
			if currentDisposition == disposition &&
				currentDuplicateOf == duplicateOf &&
				currentReason == reason {
				result.UnchangedCount++
				continue
			}
			changedIDs = append(changedIDs, risk.ID)
			updatedRisks = append(updatedRisks, risk)
		}
		if len(changedIDs) == 0 {
			return nil
		}

		update := map[string]any{
			"disposition":          disposition,
			"duplicate_of_risk_id": uint(duplicateOf),
			"tag_reason":           reason,
			"updated_at":           time.Now(),
		}
		updateDB := tx.Model(&schema.Risk{}).Where("id IN (?)", changedIDs).Updates(update)
		if updateDB.Error != nil {
			return utils.Wrap(updateDB.Error, "update risk dispositions")
		}
		result.UpdatedCount = len(changedIDs)
		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, risk := range updatedRisks {
		schema.PublishRuntimeScopedBroadcast(
			schema.RuntimeScopedBroadcastTypeRisk,
			risk.RuntimeId,
			"update",
			risk.ID,
		)
	}
	return result, nil
}

func normalizeRiskDispositionIDs(ids []int64) ([]int64, error) {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("risk ID must be positive: %d", id)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	if len(result) == 0 {
		return nil, utils.Error("at least one risk ID is required")
	}
	return result, nil
}
