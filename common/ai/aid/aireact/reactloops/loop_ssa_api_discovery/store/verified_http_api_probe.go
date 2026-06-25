package store

import (
	"github.com/yaklang/yaklang/common/utils"
)

// ListVerifiedHttpApisForProbe returns verified=true rows with a non-empty full_sample_url.
func (r *Repository) ListVerifiedHttpApisForProbe(sessionID uint) ([]VerifiedHttpApi, error) {
	if r == nil || r.db == nil {
		return nil, utils.Error("nil repository")
	}
	var rows []VerifiedHttpApi
	err := r.db.Where("session_id = ? AND verified = ? AND full_sample_url != '' AND full_sample_url IS NOT NULL",
		sessionID, true).Order("id asc").Find(&rows).Error
	return rows, err
}

// VerifiedHttpApiGateCounts holds counts for Phase1 gate and post-Phase1 quality warnings.
type VerifiedHttpApiGateCounts struct {
	Total    int
	Verified int
	Rejected int
}

// CountVerifiedHttpApiGate returns total rows, probe-ready verified (verified=true with full_sample_url), and the rest as rejected.
func (r *Repository) CountVerifiedHttpApiGate(sessionID uint) (VerifiedHttpApiGateCounts, error) {
	var out VerifiedHttpApiGateCounts
	if r == nil || r.db == nil {
		return out, utils.Error("nil repository")
	}
	total, _, err := r.CountVerifiedHttpApis(sessionID)
	if err != nil {
		return out, err
	}
	out.Total = total
	probeRows, err := r.ListVerifiedHttpApisForProbe(sessionID)
	if err != nil {
		return out, err
	}
	out.Verified = len(probeRows)
	out.Rejected = total - out.Verified
	return out, nil
}
