package store

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *Repository) UpsertEndpointVulnProbe(row *EndpointVulnProbe) error {
	if r == nil || row == nil {
		return utils.Error("nil row")
	}
	var existing EndpointVulnProbe
	err := r.db.Where("session_id = ? AND verified_http_api_id = ? AND vuln_type = ?",
		row.SessionID, row.VerifiedHttpApiID, row.VulnType).First(&existing).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return r.db.Create(row).Error
		}
		return err
	}
	row.ID = existing.ID
	return r.db.Save(row).Error
}

func (r *Repository) ListEndpointVulnProbes(sessionID, verifiedHttpApiID uint) ([]EndpointVulnProbe, error) {
	var rows []EndpointVulnProbe
	q := r.db.Where("session_id = ?", sessionID)
	if verifiedHttpApiID > 0 {
		q = q.Where("verified_http_api_id = ?", verifiedHttpApiID)
	}
	err := q.Order("verified_http_api_id asc, vuln_type asc").Find(&rows).Error
	return rows, err
}

func (r *Repository) CountEndpointVulnProbes(sessionID, verifiedHttpApiID uint) (int, error) {
	var n int
	q := r.db.Model(&EndpointVulnProbe{}).Where("session_id = ?", sessionID)
	if verifiedHttpApiID > 0 {
		q = q.Where("verified_http_api_id = ?", verifiedHttpApiID)
	}
	err := q.Count(&n).Error
	return n, err
}
