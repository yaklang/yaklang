package store

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

// Repository offers CRUD for discovery store entities.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	if db == nil {
		return nil
	}
	return &Repository{db: db}
}

func (r *Repository) DB() *gorm.DB {
	return r.db
}

// --- Session ---

func (r *Repository) CreateSession(s *DiscoverySession) error {
	if r == nil || r.db == nil {
		return utils.Error("nil repository")
	}
	return r.db.Create(s).Error
}

func (r *Repository) GetSessionByUUID(uuid string) (*DiscoverySession, error) {
	var s DiscoverySession
	err := r.db.Where("uuid = ?", uuid).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetLatestSession returns the session row most recently updated (for follow-up turns without Code path).
func (r *Repository) GetLatestSession() (*DiscoverySession, error) {
	if r == nil || r.db == nil {
		return nil, utils.Error("nil repository")
	}
	var s DiscoverySession
	err := r.db.Order("updated_at desc").First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) GetSessionByID(id uint) (*DiscoverySession, error) {
	var s DiscoverySession
	err := r.db.First(&s, id).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) UpdateSession(s *DiscoverySession) error {
	return r.db.Save(s).Error
}

func (r *Repository) UpdateSessionFields(uuid string, fields map[string]interface{}) error {
	return r.db.Model(&DiscoverySession{}).Where("uuid = ?", uuid).Updates(fields).Error
}

// --- ArchitectureComponent ---

func (r *Repository) CreateComponent(row *ArchitectureComponent) error {
	return r.db.Create(row).Error
}

func (r *Repository) UpdateComponent(row *ArchitectureComponent) error {
	return r.db.Save(row).Error
}

func (r *Repository) DeleteComponent(sessionID uint, id uint) error {
	return r.db.Where("session_id = ? AND id = ?", sessionID, id).Delete(&ArchitectureComponent{}).Error
}

func (r *Repository) GetComponent(sessionID, id uint) (*ArchitectureComponent, error) {
	var row ArchitectureComponent
	err := r.db.Where("session_id = ? AND id = ?", sessionID, id).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) ListComponents(sessionID uint) ([]ArchitectureComponent, error) {
	var rows []ArchitectureComponent
	err := r.db.Where("session_id = ?", sessionID).Order("id asc").Find(&rows).Error
	return rows, err
}

// --- ConfigArtifact ---

func (r *Repository) CreateConfigArtifact(row *ConfigArtifact) error {
	return r.db.Create(row).Error
}

func (r *Repository) UpdateConfigArtifact(row *ConfigArtifact) error {
	return r.db.Save(row).Error
}

func (r *Repository) DeleteConfigArtifact(sessionID, id uint) error {
	return r.db.Where("session_id = ? AND id = ?", sessionID, id).Delete(&ConfigArtifact{}).Error
}

func (r *Repository) GetConfigArtifact(sessionID, id uint) (*ConfigArtifact, error) {
	var row ConfigArtifact
	err := r.db.Where("session_id = ? AND id = ?", sessionID, id).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) ListConfigArtifacts(sessionID uint) ([]ConfigArtifact, error) {
	var rows []ConfigArtifact
	err := r.db.Where("session_id = ?", sessionID).Order("id asc").Find(&rows).Error
	return rows, err
}

// --- DependencyRef ---

func (r *Repository) CreateDependency(row *DependencyRef) error {
	return r.db.Create(row).Error
}

func (r *Repository) UpdateDependency(row *DependencyRef) error {
	return r.db.Save(row).Error
}

func (r *Repository) DeleteDependency(sessionID, id uint) error {
	return r.db.Where("session_id = ? AND id = ?", sessionID, id).Delete(&DependencyRef{}).Error
}

func (r *Repository) DeleteDependenciesBySession(sessionID uint) error {
	return r.db.Where("session_id = ?", sessionID).Delete(&DependencyRef{}).Error
}

func (r *Repository) ListDependencies(sessionID uint) ([]DependencyRef, error) {
	var rows []DependencyRef
	err := r.db.Where("session_id = ?", sessionID).Order("id asc").Find(&rows).Error
	return rows, err
}

// --- HttpEndpoint ---

func (r *Repository) CreateHttpEndpoint(row *HttpEndpoint) error {
	return r.db.Create(row).Error
}

func (r *Repository) UpdateHttpEndpoint(row *HttpEndpoint) error {
	return r.db.Save(row).Error
}

func (r *Repository) DeleteHttpEndpoint(sessionID, id uint) error {
	return r.db.Where("session_id = ? AND id = ?", sessionID, id).Delete(&HttpEndpoint{}).Error
}

func (r *Repository) GetHttpEndpoint(sessionID, id uint) (*HttpEndpoint, error) {
	var row HttpEndpoint
	err := r.db.Where("session_id = ? AND id = ?", sessionID, id).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) ListHttpEndpoints(sessionID uint) ([]HttpEndpoint, error) {
	var rows []HttpEndpoint
	err := r.db.Where("session_id = ?", sessionID).Order("id asc").Find(&rows).Error
	return rows, err
}

// ListAliveHttpEndpoints returns only endpoints with Status=alive (or empty/pending_validation for backward compat with old sessions).
func (r *Repository) ListAliveHttpEndpoints(sessionID uint) ([]HttpEndpoint, error) {
	var rows []HttpEndpoint
	err := r.db.Where("session_id = ? AND (status = ? OR status = ? OR status = '' OR status IS NULL)",
		sessionID, EndpointStatusAlive, EndpointStatusPendingValidation).Order("id asc").Find(&rows).Error
	return rows, err
}

// ListHttpEndpointsByStatus returns endpoints matching a specific status.
func (r *Repository) ListHttpEndpointsByStatus(sessionID uint, status string) ([]HttpEndpoint, error) {
	var rows []HttpEndpoint
	err := r.db.Where("session_id = ? AND status = ?", sessionID, status).Order("id asc").Find(&rows).Error
	return rows, err
}

// UpdateHttpEndpointStatus updates validation-related fields on an endpoint.
func (r *Repository) UpdateHttpEndpointStatus(ep *HttpEndpoint) error {
	return r.db.Model(ep).Updates(map[string]interface{}{
		"status":            ep.Status,
		"last_probed_at":    ep.LastProbedAt,
		"probe_status_code": ep.ProbeStatusCode,
		"probe_evidence":    ep.ProbeEvidence,
		"reject_reason":     ep.RejectReason,
		"function_score":    ep.FunctionScore,
	}).Error
}

// --- SecurityMechanism ---

func (r *Repository) CreateSecurityMechanism(row *SecurityMechanism) error {
	return r.db.Create(row).Error
}

func (r *Repository) UpdateSecurityMechanism(row *SecurityMechanism) error {
	return r.db.Save(row).Error
}

func (r *Repository) DeleteSecurityMechanism(sessionID, id uint) error {
	return r.db.Where("session_id = ? AND id = ?", sessionID, id).Delete(&SecurityMechanism{}).Error
}

func (r *Repository) GetSecurityMechanism(sessionID, id uint) (*SecurityMechanism, error) {
	var row SecurityMechanism
	err := r.db.Where("session_id = ? AND id = ?", sessionID, id).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) ListSecurityMechanisms(sessionID uint) ([]SecurityMechanism, error) {
	var rows []SecurityMechanism
	err := r.db.Where("session_id = ?", sessionID).Order("id asc").Find(&rows).Error
	return rows, err
}

// --- BusinessCapability ---

func (r *Repository) CreateBusinessCapability(row *BusinessCapability) error {
	return r.db.Create(row).Error
}

func (r *Repository) UpdateBusinessCapability(row *BusinessCapability) error {
	return r.db.Save(row).Error
}

func (r *Repository) DeleteBusinessCapability(sessionID, id uint) error {
	return r.db.Where("session_id = ? AND id = ?", sessionID, id).Delete(&BusinessCapability{}).Error
}

func (r *Repository) GetBusinessCapability(sessionID, id uint) (*BusinessCapability, error) {
	var row BusinessCapability
	err := r.db.Where("session_id = ? AND id = ?", sessionID, id).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) ListBusinessCapabilities(sessionID uint) ([]BusinessCapability, error) {
	var rows []BusinessCapability
	err := r.db.Where("session_id = ?", sessionID).Order("id asc").Find(&rows).Error
	return rows, err
}

// --- DiscoveryEvent ---

func (r *Repository) AppendEvent(sessionID uint, level, message, payloadJSON string) error {
	ev := &DiscoveryEvent{
		SessionID:   sessionID,
		Level:       level,
		Message:     message,
		PayloadJSON: payloadJSON,
	}
	return r.db.Create(ev).Error
}

func (r *Repository) ListEvents(sessionID uint, limit int) ([]DiscoveryEvent, error) {
	var rows []DiscoveryEvent
	q := r.db.Where("session_id = ?", sessionID).Order("id desc")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&rows).Error
	return rows, err
}

func (r *Repository) CountEvents(sessionID uint) (int64, error) {
	if r == nil || r.db == nil {
		return 0, utils.Error("nil repository")
	}
	var n int64
	err := r.db.Model(&DiscoveryEvent{}).Where("session_id = ?", sessionID).Count(&n).Error
	return n, err
}

// CountsBySession returns table row counts for status summaries.
func (r *Repository) CountsBySession(sessionID uint) (components, configs, deps, endpoints, sec, biz, verified, sfFindings, vulnVer int, err error) {
	var n int
	err = r.db.Model(&ArchitectureComponent{}).Where("session_id = ?", sessionID).Count(&n).Error
	if err != nil {
		return
	}
	components = n

	err = r.db.Model(&ConfigArtifact{}).Where("session_id = ?", sessionID).Count(&n).Error
	if err != nil {
		return
	}
	configs = n

	err = r.db.Model(&DependencyRef{}).Where("session_id = ?", sessionID).Count(&n).Error
	if err != nil {
		return
	}
	deps = n

	err = r.db.Model(&HttpEndpoint{}).Where("session_id = ?", sessionID).Count(&n).Error
	if err != nil {
		return
	}
	endpoints = n

	err = r.db.Model(&SecurityMechanism{}).Where("session_id = ?", sessionID).Count(&n).Error
	if err != nil {
		return
	}
	sec = n

	err = r.db.Model(&BusinessCapability{}).Where("session_id = ?", sessionID).Count(&n).Error
	if err != nil {
		return
	}
	biz = n

	err = r.db.Model(&VerifiedEndpoint{}).Where("session_id = ?", sessionID).Count(&n).Error
	if err != nil {
		return
	}
	verified = n

	err = r.db.Model(&DiscoverySyntaxFlowFinding{}).Where("session_id = ?", sessionID).Count(&n).Error
	if err != nil {
		return
	}
	sfFindings = n

	err = r.db.Model(&VulnVerification{}).Where("session_id = ?", sessionID).Count(&n).Error
	if err != nil {
		return
	}
	vulnVer = n
	return
}

// --- VerifiedHttpApi (Phase1) ---

func (r *Repository) CreateVerifiedHttpApi(row *VerifiedHttpApi) error {
	return r.db.Create(row).Error
}

func (r *Repository) UpdateVerifiedHttpApi(row *VerifiedHttpApi) error {
	return r.db.Save(row).Error
}

func (r *Repository) UpsertVerifiedHttpApi(row *VerifiedHttpApi) error {
	if r == nil || r.db == nil || row == nil {
		return utils.Error("nil repository or row")
	}
	var existing VerifiedHttpApi
	err := r.db.Where("session_id = ? AND method = ? AND path_pattern = ?",
		row.SessionID, row.Method, row.PathPattern).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return r.CreateVerifiedHttpApi(row)
	}
	if err != nil {
		return err
	}
	row.ID = existing.ID
	row.CreatedAt = existing.CreatedAt
	merged := MergeVerifiedHttpApiUpdate(&existing, row)
	return r.UpdateVerifiedHttpApi(merged)
}

func (r *Repository) GetVerifiedHttpApi(sessionID, id uint) (*VerifiedHttpApi, error) {
	var row VerifiedHttpApi
	err := r.db.Where("session_id = ? AND id = ?", sessionID, id).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) ListVerifiedHttpApis(sessionID uint) ([]VerifiedHttpApi, error) {
	var rows []VerifiedHttpApi
	err := r.db.Where("session_id = ?", sessionID).Order("id asc").Find(&rows).Error
	return rows, err
}

func (r *Repository) ListVerifiedHttpApisWhereVerified(sessionID uint, verified bool) ([]VerifiedHttpApi, error) {
	var rows []VerifiedHttpApi
	err := r.db.Where("session_id = ? AND verified = ?", sessionID, verified).Order("id asc").Find(&rows).Error
	return rows, err
}

func (r *Repository) CountVerifiedHttpApis(sessionID uint) (total int, verified int, err error) {
	if r == nil || r.db == nil {
		return 0, 0, utils.Error("nil repository")
	}
	var n int
	if err = r.db.Model(&VerifiedHttpApi{}).Where("session_id = ?", sessionID).Count(&n).Error; err != nil {
		return
	}
	total = n
	if err = r.db.Model(&VerifiedHttpApi{}).Where("session_id = ? AND verified = ?", sessionID, true).Count(&n).Error; err != nil {
		return
	}
	verified = n
	return
}

func (r *Repository) DeleteVerifiedHttpApisBySession(sessionID uint) error {
	return r.db.Where("session_id = ?", sessionID).Delete(&VerifiedHttpApi{}).Error
}

// --- VerifiedEndpoint ---

func (r *Repository) DeleteVerifiedEndpointsBySession(sessionID uint) error {
	return r.db.Where("session_id = ?", sessionID).Delete(&VerifiedEndpoint{}).Error
}

func (r *Repository) ListVerifiedEndpoints(sessionID uint) ([]VerifiedEndpoint, error) {
	var rows []VerifiedEndpoint
	err := r.db.Where("session_id = ?", sessionID).Order("id asc").Find(&rows).Error
	return rows, err
}

// --- DiscoverySyntaxFlowFinding ---

func (r *Repository) DeleteDiscoverySyntaxFlowFindingsBySession(sessionID uint) error {
	return r.db.Where("session_id = ?", sessionID).Delete(&DiscoverySyntaxFlowFinding{}).Error
}

func (r *Repository) CreateDiscoverySyntaxFlowFinding(row *DiscoverySyntaxFlowFinding) error {
	return r.db.Create(row).Error
}

func (r *Repository) ListDiscoverySyntaxFlowFindings(sessionID uint) ([]DiscoverySyntaxFlowFinding, error) {
	var rows []DiscoverySyntaxFlowFinding
	err := r.db.Where("session_id = ?", sessionID).Order("id asc").Find(&rows).Error
	return rows, err
}

func (r *Repository) GetDiscoverySyntaxFlowFinding(sessionID, id uint) (*DiscoverySyntaxFlowFinding, error) {
	var row DiscoverySyntaxFlowFinding
	err := r.db.Where("session_id = ? AND id = ?", sessionID, id).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// --- VulnVerification ---

func (r *Repository) DeleteVulnVerificationsBySession(sessionID uint) error {
	return r.db.Where("session_id = ?", sessionID).Delete(&VulnVerification{}).Error
}

func (r *Repository) CreateVulnVerification(row *VulnVerification) error {
	return r.db.Create(row).Error
}

func (r *Repository) UpdateVulnVerification(row *VulnVerification) error {
	return r.db.Save(row).Error
}

func (r *Repository) ListVulnVerifications(sessionID uint) ([]VulnVerification, error) {
	var rows []VulnVerification
	err := r.db.Where("session_id = ?", sessionID).Order("id asc").Find(&rows).Error
	return rows, err
}

func (r *Repository) GetVulnVerification(sessionID, id uint) (*VulnVerification, error) {
	var row VulnVerification
	err := r.db.Where("session_id = ? AND id = ?", sessionID, id).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) GetVulnVerificationByDynamicFindingID(sessionID, dynamicFindingID uint) (*VulnVerification, error) {
	if dynamicFindingID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var row VulnVerification
	err := r.db.Where("session_id = ? AND dynamic_finding_id = ?", sessionID, dynamicFindingID).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) UpsertVulnVerificationByDynamicFinding(row *VulnVerification) error {
	if row == nil {
		return nil
	}
	if row.Source == "" {
		row.Source = "dynamic"
	}
	var existing VulnVerification
	err := r.db.Where("session_id = ? AND dynamic_finding_id = ?", row.SessionID, row.DynamicFindingID).First(&existing).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return r.CreateVulnVerification(row)
		}
		return err
	}
	row.ID = existing.ID
	row.CreatedAt = existing.CreatedAt
	return r.UpdateVulnVerification(row)
}

// --- AuthCredential ---

func (r *Repository) CreateAuthCredential(row *AuthCredential) error {
	return r.db.Create(row).Error
}

func (r *Repository) UpdateAuthCredential(row *AuthCredential) error {
	return r.db.Save(row).Error
}

func (r *Repository) GetAuthCredential(sessionID, id uint) (*AuthCredential, error) {
	var row AuthCredential
	err := r.db.Where("session_id = ? AND id = ?", sessionID, id).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) ListAuthCredentials(sessionID uint) ([]AuthCredential, error) {
	var rows []AuthCredential
	err := r.db.Where("session_id = ?", sessionID).Order("id asc").Find(&rows).Error
	return rows, err
}

func (r *Repository) ListVerifiedAuthCredentials(sessionID uint) ([]AuthCredential, error) {
	var rows []AuthCredential
	err := r.db.Where("session_id = ? AND verified = ?", sessionID, true).Order("id asc").Find(&rows).Error
	return rows, err
}

func (r *Repository) DeleteAuthCredentialsBySession(sessionID uint) error {
	return r.db.Where("session_id = ?", sessionID).Delete(&AuthCredential{}).Error
}

// GetFreshestVerifiedCredential returns the most recently verified credential with fresh state.
func (r *Repository) GetFreshestVerifiedCredential(sessionID uint) (*AuthCredential, error) {
	var row AuthCredential
	err := r.db.Where("session_id = ? AND verified = ?", sessionID, true).
		Order("last_verified_at DESC, id DESC").First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// --- AuthAcquisitionRecipe ---

func (r *Repository) CreateAuthAcquisitionRecipe(row *AuthAcquisitionRecipe) error {
	return r.db.Create(row).Error
}

func (r *Repository) UpdateAuthAcquisitionRecipe(row *AuthAcquisitionRecipe) error {
	return r.db.Save(row).Error
}

func (r *Repository) GetAuthAcquisitionRecipe(sessionID, id uint) (*AuthAcquisitionRecipe, error) {
	var row AuthAcquisitionRecipe
	err := r.db.Where("session_id = ? AND id = ?", sessionID, id).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) GetRecipeByCredentialID(sessionID, credentialID uint) (*AuthAcquisitionRecipe, error) {
	var row AuthAcquisitionRecipe
	err := r.db.Where("session_id = ? AND credential_id = ?", sessionID, credentialID).
		Order("id DESC").First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) ListAuthAcquisitionRecipes(sessionID uint) ([]AuthAcquisitionRecipe, error) {
	var rows []AuthAcquisitionRecipe
	err := r.db.Where("session_id = ?", sessionID).Order("id asc").Find(&rows).Error
	return rows, err
}

// --- EndpointValidationAttempt ---

func (r *Repository) CreateEndpointValidationAttempt(row *EndpointValidationAttempt) error {
	return r.db.Create(row).Error
}

func (r *Repository) ListEndpointValidationAttempts(sessionID, endpointID uint) ([]EndpointValidationAttempt, error) {
	var rows []EndpointValidationAttempt
	err := r.db.Where("session_id = ? AND http_endpoint_id = ?", sessionID, endpointID).
		Order("id asc").Find(&rows).Error
	return rows, err
}

func (r *Repository) ListEndpointValidationAttemptsBySession(sessionID uint, limit int) ([]EndpointValidationAttempt, error) {
	if r == nil || r.db == nil {
		return nil, utils.Error("nil repository")
	}
	var rows []EndpointValidationAttempt
	q := r.db.Where("session_id = ?", sessionID).Order("id desc")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&rows).Error
	return rows, err
}

func (r *Repository) CountEndpointValidationAttempts(sessionID, endpointID uint) (int, error) {
	var n int
	err := r.db.Model(&EndpointValidationAttempt{}).
		Where("session_id = ? AND http_endpoint_id = ?", sessionID, endpointID).Count(&n).Error
	return n, err
}

// --- DynamicVulnFinding ---

func (r *Repository) CreateDynamicVulnFinding(row *DynamicVulnFinding) error {
	return r.db.Create(row).Error
}

func (r *Repository) UpdateDynamicVulnFinding(row *DynamicVulnFinding) error {
	return r.db.Save(row).Error
}

func (r *Repository) GetDynamicVulnFinding(sessionID, id uint) (*DynamicVulnFinding, error) {
	var row DynamicVulnFinding
	err := r.db.Where("session_id = ? AND id = ?", sessionID, id).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) ListDynamicVulnFindings(sessionID uint) ([]DynamicVulnFinding, error) {
	var rows []DynamicVulnFinding
	err := r.db.Where("session_id = ?", sessionID).Order("id asc").Find(&rows).Error
	return rows, err
}

func (r *Repository) ListDynamicVulnFindingsByStatus(sessionID uint, status string) ([]DynamicVulnFinding, error) {
	var rows []DynamicVulnFinding
	err := r.db.Where("session_id = ? AND status = ?", sessionID, status).Order("id asc").Find(&rows).Error
	return rows, err
}

func (r *Repository) DeleteDynamicVulnFindingsBySession(sessionID uint) error {
	return r.db.Where("session_id = ?", sessionID).Delete(&DynamicVulnFinding{}).Error
}

// --- CoverageWorkItem (legacy read-only; historical sessions) ---

// CountCoverageWorkItemsByStatus counts rows for session + kind + status.
func (r *Repository) CountCoverageWorkItemsByStatus(sessionID uint, kind, status string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, utils.Error("nil repository")
	}
	var n int64
	err := r.db.Model(&CoverageWorkItem{}).Where("session_id = ? AND kind = ? AND status = ?", sessionID, kind, status).Count(&n).Error
	return n, err
}

// CountCoverageWorkItems counts all rows for session + kind.
func (r *Repository) CountCoverageWorkItems(sessionID uint, kind string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, utils.Error("nil repository")
	}
	var n int64
	err := r.db.Model(&CoverageWorkItem{}).Where("session_id = ? AND kind = ?", sessionID, kind).Count(&n).Error
	return n, err
}

// ListPendingHttpEndpointCoverageLabels returns ref_label for pending http_endpoint items (for prompts).
func (r *Repository) ListPendingHttpEndpointCoverageLabels(sessionID uint, limit int) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, utils.Error("nil repository")
	}
	var rows []CoverageWorkItem
	q := r.db.Where("session_id = ? AND kind = ? AND status = ?", sessionID, CoverageKindHttpEndpoint, CoverageStatusPending).Order("id asc")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, w := range rows {
		out = append(out, w.RefLabel)
	}
	return out, nil
}

// ListCoverageWorkItems returns coverage rows for a session (optional kind filter).
func (r *Repository) ListCoverageWorkItems(sessionID uint, kind string, limit int) ([]CoverageWorkItem, error) {
	if r == nil || r.db == nil {
		return nil, utils.Error("nil repository")
	}
	var rows []CoverageWorkItem
	q := r.db.Where("session_id = ?", sessionID)
	if strings.TrimSpace(kind) != "" {
		q = q.Where("kind = ?", kind)
	}
	q = q.Order("id asc")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&rows).Error
	return rows, err
}

// --- VulnChecklistItem ---

func (r *Repository) ReplaceVulnChecklistItems(sessionID uint, items []VulnChecklistItem) error {
	if r == nil || r.db == nil {
		return utils.Error("nil repository")
	}
	tx := r.db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	if err := tx.Where("session_id = ?", sessionID).Delete(&VulnChecklistItem{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	for i := range items {
		items[i].SessionID = sessionID
		if items[i].Status == "" {
			items[i].Status = VulnChecklistStatusPending
		}
		if err := tx.Create(&items[i]).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit().Error
}

func (r *Repository) ListVulnChecklistItems(sessionID uint) ([]VulnChecklistItem, error) {
	if r == nil || r.db == nil {
		return nil, utils.Error("nil repository")
	}
	var rows []VulnChecklistItem
	err := r.db.Where("session_id = ?", sessionID).Order("priority desc, id asc").Find(&rows).Error
	return rows, err
}

func (r *Repository) CountVulnChecklistByConfidence(sessionID uint) (high, medium, low, none int64, err error) {
	if r == nil || r.db == nil {
		return 0, 0, 0, 0, utils.Error("nil repository")
	}
	type row struct {
		AssocConfidence string
		N               int64
	}
	var rows []row
	err = r.db.Model(&VulnChecklistItem{}).
		Select("assoc_confidence, count(*) as n").
		Where("session_id = ?", sessionID).
		Group("assoc_confidence").
		Scan(&rows).Error
	if err != nil {
		return
	}
	for _, x := range rows {
		switch strings.ToLower(strings.TrimSpace(x.AssocConfidence)) {
		case "high":
			high = x.N
		case "medium":
			medium = x.N
		case "low":
			low = x.N
		default:
			none += x.N
		}
	}
	return
}

// --- PhaseArtifact ---

func (r *Repository) UpsertPhaseArtifact(sessionID uint, kind, payloadJSON string) error {
	if r == nil || r.db == nil {
		return utils.Error("nil repository")
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return utils.Error("artifact kind required")
	}
	var existing PhaseArtifact
	err := r.db.Where("session_id = ? AND kind = ?", sessionID, kind).First(&existing).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return r.db.Create(&PhaseArtifact{
				SessionID:   sessionID,
				Kind:        kind,
				Version:     1,
				PayloadJSON: payloadJSON,
			}).Error
		}
		return err
	}
	existing.PayloadJSON = payloadJSON
	existing.Version++
	return r.db.Save(&existing).Error
}

func (r *Repository) GetPhaseArtifact(sessionID uint, kind string) (*PhaseArtifact, error) {
	if r == nil || r.db == nil {
		return nil, utils.Error("nil repository")
	}
	var row PhaseArtifact
	err := r.db.Where("session_id = ? AND kind = ?", sessionID, strings.TrimSpace(kind)).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) ListPhaseArtifactKinds(sessionID uint) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, utils.Error("nil repository")
	}
	var kinds []string
	err := r.db.Model(&PhaseArtifact{}).Where("session_id = ?", sessionID).Order("kind asc").Pluck("kind", &kinds).Error
	return kinds, err
}

func (r *Repository) ListPhaseArtifacts(sessionID uint, kind string, limit int) ([]PhaseArtifact, error) {
	if r == nil || r.db == nil {
		return nil, utils.Error("nil repository")
	}
	var rows []PhaseArtifact
	q := r.db.Where("session_id = ?", sessionID)
	if strings.TrimSpace(kind) != "" {
		q = q.Where("kind = ?", strings.TrimSpace(kind))
	}
	q = q.Order("kind asc")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&rows).Error
	return rows, err
}

func (r *Repository) CountPhaseArtifacts(sessionID uint) (int64, error) {
	if r == nil || r.db == nil {
		return 0, utils.Error("nil repository")
	}
	var n int64
	err := r.db.Model(&PhaseArtifact{}).Where("session_id = ?", sessionID).Count(&n).Error
	return n, err
}
