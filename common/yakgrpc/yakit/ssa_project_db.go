package yakit

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaproject"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const currentSSAProjectIDKey = "current_ssa_project_id"

// ResolveDefaultSSADatabasePath returns the configured legacy/default SSA IR database path.
func ResolveDefaultSSADatabasePath() string {
	return consts.GetCanonicalDefaultSSADatabasePath()
}

// ResolveSSAProjectDatabasePath returns the SSA DB path for a project.
// Legacy projects with empty DatabasePath use the default SSA database.
func ResolveSSAProjectDatabasePath(p *schema.SSAProject) string {
	if p == nil {
		return ResolveDefaultSSADatabasePath()
	}
	if path := filepath.Clean(p.DatabasePath); path != "" && path != "." {
		return p.DatabasePath
	}
	return ResolveDefaultSSADatabasePath()
}

// IsDefaultSSADatabasePath reports whether path is the process default SSA database.
func IsDefaultSSADatabasePath(path string) bool {
	if path == "" {
		return true
	}
	absPath, err1 := filepath.Abs(path)
	absDefault, err2 := filepath.Abs(ResolveDefaultSSADatabasePath())
	if err1 != nil || err2 != nil {
		return path == ResolveDefaultSSADatabasePath()
	}
	return absPath == absDefault
}

// OpenSSAProjectDatabaseRaw switches the global SSA IR database connection to raw.
// Other cached SSA database handles are not closed; sqlite files are not deleted.
func OpenSSAProjectDatabaseRaw(raw string) error {
	if raw == "" {
		return utils.Errorf("open SSA database failed: path is empty")
	}
	consts.SetSSADatabaseInfo(raw)
	return consts.SetGormSSAProjectDatabaseByInfo(raw)
}

// OpenSSAProjectDatabase switches SSA connection to the database bound to the project.
func OpenSSAProjectDatabase(p *schema.SSAProject) error {
	if p == nil {
		return utils.Errorf("open SSA project database failed: project is nil")
	}
	return OpenSSAProjectDatabaseRaw(ResolveSSAProjectDatabasePath(p))
}

// SetCurrentSSAProjectID records the active SSA analysis project in profile storage.
// projectID == 0 means no project is selected and clears the stored value.
func SetCurrentSSAProjectID(profileDB *gorm.DB, projectID uint64) {
	if profileDB == nil {
		profileDB = consts.GetGormProfileDatabase()
	}
	if profileDB == nil {
		return
	}
	if projectID == 0 {
		clearCurrentSSAProjectID(profileDB)
		return
	}
	_ = SetKey(profileDB, currentSSAProjectIDKey, strconv.FormatUint(projectID, 10))
}

func clearCurrentSSAProjectID(profileDB *gorm.DB) {
	if profileDB == nil {
		profileDB = consts.GetGormProfileDatabase()
	}
	if profileDB != nil {
		DelKey(profileDB, currentSSAProjectIDKey)
	}
}

func clearCurrentSSAProjectIDIfMatch(projectID uint64) {
	if GetCurrentSSAProjectID() == projectID {
		clearCurrentSSAProjectID(consts.GetGormProfileDatabase())
	}
}

func isSSAProjectNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "record not found")
}

// GetCurrentSSAProjectID returns the active SSA analysis project id, or 0 if unset / unselected.
func GetCurrentSSAProjectID() uint64 {
	raw := GetKey(consts.GetGormProfileDatabase(), currentSSAProjectIDKey)
	if raw == "" || raw == "0" { // "0" kept for legacy profile values written before DelKey-based clear
		return 0
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// EnsureSSAProjectDatabaseOpen switches to the project's database when projectID is set.
func EnsureSSAProjectDatabaseOpen(projectID uint64) error {
	if projectID == 0 {
		clearCurrentSSAProjectID(consts.GetGormProfileDatabase())
		return ensureDefaultSSADatabaseOpen()
	}
	project, err := GetSSAProjectById(projectID)
	if err != nil {
		if isSSAProjectNotFound(err) {
			clearCurrentSSAProjectIDIfMatch(projectID)
			return ensureDefaultSSADatabaseOpen()
		}
		return err
	}
	expectedPath := ResolveSSAProjectDatabasePath(project)
	if GetCurrentSSAProjectID() == projectID &&
		consts.IsGormSSAProjectDatabaseOpen() &&
		isSSADatabasePathActive(expectedPath) {
		return nil
	}
	if err := OpenSSAProjectDatabase(project); err != nil {
		return err
	}
	SetCurrentSSAProjectID(consts.GetGormProfileDatabase(), projectID)
	return nil
}

// EnsureSSAProjectDatabaseReady opens the SSA IR database for the current request context.
// A single non-zero project id opens that project; otherwise the active SSA project or default is used.
func EnsureSSAProjectDatabaseReady(projectIDs ...uint64) error {
	if len(projectIDs) == 1 && projectIDs[0] > 0 {
		return EnsureSSAProjectDatabaseOpen(projectIDs[0])
	}
	if id := GetCurrentSSAProjectID(); id > 0 {
		return EnsureSSAProjectDatabaseOpen(id)
	}
	return ensureDefaultSSADatabaseOpen()
}

// ssaDatabaseSession captures the global SSA IR database switch state for restore.
type ssaDatabaseSession struct {
	activePath string
	projectID  uint64
}

func captureSSADatabaseSession() ssaDatabaseSession {
	return ssaDatabaseSession{
		activePath: consts.GetActiveSSADatabaseRawPath(),
		projectID:  GetCurrentSSAProjectID(),
	}
}

func restoreSSADatabaseSession(sess ssaDatabaseSession) error {
	if sess.projectID > 0 {
		return EnsureSSAProjectDatabaseOpen(sess.projectID)
	}
	if sess.activePath == "" || IsDefaultSSADatabasePath(sess.activePath) {
		clearCurrentSSAProjectID(consts.GetGormProfileDatabase())
		return ensureDefaultSSADatabaseOpen()
	}
	if err := OpenSSAProjectDatabaseRaw(sess.activePath); err != nil {
		return err
	}
	clearCurrentSSAProjectID(consts.GetGormProfileDatabase())
	return nil
}

// LookupSSAProjectIDByProgramName finds the SSA analysis project that owns programName.
// The global SSA database handle is restored to the pre-lookup state before returning.
func LookupSSAProjectIDByProgramName(profileDB *gorm.DB, programName string) (uint64, error) {
	if programName == "" {
		return 0, utils.Errorf("lookup SSA project by program name failed: program name is empty")
	}
	sess := captureSSADatabaseSession()
	defer func() { _ = restoreSSADatabaseSession(sess) }()

	if profileDB == nil {
		profileDB = consts.GetGormProfileDatabase()
	}
	var projects []schema.SSAProject
	if err := profileDB.Find(&projects).Error; err != nil {
		return 0, utils.Errorf("lookup SSA project by program name failed: %s", err)
	}
	for i := range projects {
		p := &projects[i]
		if err := OpenSSAProjectDatabase(p); err != nil {
			continue
		}
		irProg, err := GetSSAProgramByName(consts.GetGormSSAProjectDataBase(), programName)
		if err == nil && irProg != nil {
			if irProg.ProjectID > 0 {
				return irProg.ProjectID, nil
			}
			return uint64(p.ID), nil
		}
	}
	if err := ensureDefaultSSADatabaseOpen(); err != nil {
		return 0, err
	}
	irProg, err := GetSSAProgramByName(consts.GetGormSSAProjectDataBase(), programName)
	if err != nil {
		return 0, utils.Errorf("lookup SSA project by program name failed: %s", err)
	}
	if irProg.ProjectID > 0 {
		return irProg.ProjectID, nil
	}
	return 0, utils.Errorf("lookup SSA project by program name failed: program has no project_id")
}

// EnsureSSAProjectDatabaseForProgramFilter opens the SSA IR DB implied by an SSAProgramFilter.
func EnsureSSAProjectDatabaseForProgramFilter(filter *ypb.SSAProgramFilter) error {
	projectIDs, multi, err := ResolveSSAProgramQueryProjectIDs(filter)
	if err != nil {
		return err
	}
	if multi {
		return nil
	}
	return ensureSSAProjectDatabaseForResolvedProjectIDs(filter, projectIDs)
}

func ensureSSAProjectDatabaseForResolvedProjectIDs(_ *ypb.SSAProgramFilter, projectIDs []uint64) error {
	if len(projectIDs) == 1 {
		return EnsureSSAProjectDatabaseForProjectID(projectIDs[0])
	}
	return EnsureSSAProjectDatabaseReady()
}

// ResolveSSAProgramQueryProjectIDs decides which SSA IR databases must be queried.
// multi is true when programs may span more than one database and results must be merged.
func ResolveSSAProgramQueryProjectIDs(filter *ypb.SSAProgramFilter) (projectIDs []uint64, multi bool, err error) {
	if filter == nil {
		return nil, false, nil
	}

	ids := uniqueUint64Slice(filter.GetProjectIds())
	if len(ids) > 1 {
		return ids, true, nil
	}
	if len(ids) == 1 {
		merge, err := ShouldMergeDefaultAndDedicatedForProject(ids[0])
		if err != nil {
			return nil, false, err
		}
		return ids, merge, nil
	}

	names := filter.GetProgramNames()
	if len(names) == 0 {
		return nil, false, nil
	}
	ids = lookupProjectIDsByProgramNames(consts.GetGormProfileDatabase(), names)
	if len(ids) == 0 {
		return nil, false, nil
	}
	if len(ids) == 1 {
		return ids, false, nil
	}
	return ids, true, nil
}

// lookupProjectIDsByProgramNames resolves program names to distinct SSA project ids (0 = default/unknown).
func lookupProjectIDsByProgramNames(profileDB *gorm.DB, names []string) []uint64 {
	idSet := make(map[uint64]struct{})
	for _, name := range names {
		if name == "" {
			continue
		}
		id, lookupErr := LookupSSAProjectIDByProgramName(profileDB, name)
		if lookupErr != nil {
			idSet[0] = struct{}{}
			continue
		}
		idSet[id] = struct{}{}
	}
	return mapKeysUint64(idSet)
}

func uniqueUint64Slice(ids []uint64) []uint64 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[uint64]struct{}, len(ids))
	out := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func mapKeysUint64(m map[uint64]struct{}) []uint64 {
	out := make([]uint64, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	return out
}

// EnsureSSAProjectDatabaseForProjectID opens the SSA IR database for a resolved project id (0 = default).
func EnsureSSAProjectDatabaseForProjectID(projectID uint64) error {
	if projectID > 0 {
		return EnsureSSAProjectDatabaseOpen(projectID)
	}
	return ensureDefaultSSADatabaseOpen()
}

func ensureDefaultSSADatabaseOpen() error {
	if consts.IsGormSSAProjectDatabaseOpen() &&
		isSSADatabasePathActive(ResolveDefaultSSADatabasePath()) {
		return nil
	}
	return OpenSSAProjectDatabaseRaw(ResolveDefaultSSADatabasePath())
}

// IsSharedSSAProfileProject reports whether the profile project uses shared IR (default/temporary).
func IsSharedSSAProfileProject(proj *schema.Project) bool {
	if proj == nil {
		return false
	}
	name := proj.ProjectName
	return name == INIT_DATABASE_RECORD_NAME || name == TEMPORARY_PROJECT_NAME
}

// GetCurrentSSAProfileProject returns the active schema.Project for IRify SSA profile selection.
func GetCurrentSSAProfileProject() (*schema.Project, error) {
	profileDB := consts.GetGormProfileDatabase()
	if profileDB == nil {
		return nil, utils.Errorf("profile database is not initialized")
	}
	return GetCurrentProject(profileDB, TypeSSAProject)
}

// IsSharedSSAProfileCurrent reports whether the active SSA profile is default or temporary shared mode.
func IsSharedSSAProfileCurrent(profileDB *gorm.DB) (bool, *schema.Project, error) {
	if profileDB == nil {
		profileDB = consts.GetGormProfileDatabase()
	}
	proj, err := GetCurrentProject(profileDB, TypeSSAProject)
	if err != nil {
		return false, nil, err
	}
	return IsSharedSSAProfileProject(proj), proj, nil
}

func normalizeSSADBPath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

func sharedPoolSSAProjectWhere(db *gorm.DB) *gorm.DB {
	defaultCands := pathCandidates(ResolveDefaultSSADatabasePath())
	if len(defaultCands) == 0 {
		return db.Where("(database_path = '' OR database_path IS NULL)")
	}
	return db.Where(
		"(database_path = '' OR database_path IS NULL) OR database_path IN (?)",
		defaultCands,
	)
}

func dedicatedPoolSSAProjectWhere(db *gorm.DB) *gorm.DB {
	db = db.Where("database_path != '' AND database_path IS NOT NULL")
	defaultCands := pathCandidates(ResolveDefaultSSADatabasePath())
	if len(defaultCands) == 0 {
		return db
	}
	return db.Where("database_path NOT IN (?)", defaultCands)
}

// ApplySSAProjectActiveDatabaseScope filters ssa_projects to the active profile/IR database context.
func ApplySSAProjectActiveDatabaseScope(db *gorm.DB, filter *ypb.SSAProjectFilter) (*gorm.DB, error) {
	if filter != nil && filter.GetDisableActiveDatabaseScope() {
		return db, nil
	}

	if filter != nil {
		switch filter.GetListPool() {
		case ypb.SSAProjectListPool_SSA_PROJECT_LIST_SHARED:
			return sharedPoolSSAProjectWhere(db), nil
		case ypb.SSAProjectListPool_SSA_PROJECT_LIST_DEDICATED:
			return dedicatedPoolSSAProjectWhere(db), nil
		}
	}

	isShared, curProj, err := IsSharedSSAProfileCurrent(nil)
	if err != nil {
		return nil, err
	}

	if isShared {
		return sharedPoolSSAProjectWhere(db), nil
	}

	// Independent profile mode: per-project dedicated sqlite files (non-empty database_path).
	active := normalizeSSADBPath(consts.GetActiveSSADatabaseRawPath())
	if active == "" && curProj != nil {
		active = normalizeSSADBPath(curProj.DatabasePath)
	}
	db = db.Where("database_path != '' AND database_path IS NOT NULL")
	if active == "" {
		return db, nil
	}
	candidates := pathCandidates(active)
	if len(candidates) == 0 {
		return db, nil
	}
	// Narrow to the active dedicated sqlite when it matches a project file; otherwise keep all dedicated rows.
	sub := consts.GetGormProfileDatabase().Model(&schema.SSAProject{}).Where("database_path IN (?)", candidates)
	var matched int64
	if err := sub.Count(&matched).Error; err == nil && matched > 0 {
		return db.Where("database_path IN (?)", candidates), nil
	}
	return db, nil
}

func pathCandidates(path string) []string {
	raw := strings.TrimSpace(path)
	if raw == "" {
		return nil
	}
	norm := normalizeSSADBPath(raw)
	clean := filepath.Clean(raw)
	uniq := make([]string, 0, 3)
	seen := make(map[string]struct{}, 3)
	for _, p := range []string{raw, norm, clean} {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		uniq = append(uniq, p)
	}
	return uniq
}

// ResolveSSAProjectDisplayDatabasePaths returns default/resolved paths for GRPC list display.
func ResolveSSAProjectDisplayDatabasePaths(p *schema.SSAProject) (defaultPath, resolvedPath string) {
	if isShared, curProj, err := IsSharedSSAProfileCurrent(nil); err == nil && isShared && !ProjectUsesDedicatedSSADB(p) {
		if curProj != nil && strings.TrimSpace(curProj.DatabasePath) != "" {
			resolvedPath = curProj.DatabasePath
		} else if active := consts.GetActiveSSADatabaseRawPath(); active != "" {
			resolvedPath = active
		} else {
			resolvedPath = ResolveDefaultSSADatabasePath()
		}
		return resolvedPath, resolvedPath
	}
	defaultPath = ResolveDefaultSSADatabasePath()
	resolvedPath = ResolveSSAProjectDatabasePath(p)
	return defaultPath, resolvedPath
}

// bindSSAProjectSharedDatabase opens the profile-linked shared IR DB without creating a dedicated file.
// Internal/shared-pool projects must keep database_path empty so ListPool=SHARED can find them.
func bindSSAProjectSharedDatabase(profileDB *gorm.DB, project *schema.SSAProject) error {
	if profileDB == nil {
		profileDB = consts.GetGormProfileDatabase()
	}
	if project == nil || project.ID == 0 {
		return utils.Errorf("bind shared SSA database failed: project is nil or has no id")
	}
	if err := profileDB.Model(project).Update("database_path", "").Error; err != nil {
		return utils.Errorf("bind shared SSA database failed: update database_path: %s", err)
	}
	project.DatabasePath = ""
	ssaproject.RefreshProjectHash(project)
	if err := profileDB.Model(project).Update("hash", project.Hash).Error; err != nil {
		return utils.Errorf("bind shared SSA database failed: update hash: %s", err)
	}

	curProj, err := GetCurrentSSAProfileProject()
	if err != nil {
		return err
	}
	raw := strings.TrimSpace(curProj.DatabasePath)
	if raw == "" {
		raw = consts.GetActiveSSADatabaseRawPath()
	}
	if raw == "" {
		return utils.Errorf("bind shared SSA database failed: active IR path is empty")
	}
	if err := OpenSSAProjectDatabaseRaw(raw); err != nil {
		return err
	}
	SetCurrentSSAProjectID(profileDB, uint64(project.ID))
	return nil
}

// BindSSAProjectDatabase creates a dedicated SSA DB for a new project and opens it.
func BindSSAProjectDatabase(profileDB *gorm.DB, project *schema.SSAProject) error {
	if profileDB == nil {
		return utils.Errorf("bind SSA project database failed: profile db is nil")
	}
	if project == nil || project.ID == 0 {
		return utils.Errorf("bind SSA project database failed: project is nil or has no id")
	}
	if project.DatabasePath == "" {
		path, err := CreateProjectFile(project.ProjectName, TypeSSAProject)
		if err != nil {
			return utils.Errorf("bind SSA project database failed: %s", err)
		}
		project.DatabasePath = path
		if err := profileDB.Model(project).Update("database_path", path).Error; err != nil {
			return utils.Errorf("bind SSA project database failed: update database_path: %s", err)
		}
	}
	ssaproject.RefreshProjectHash(project)
	if err := profileDB.Model(project).Update("hash", project.Hash).Error; err != nil {
		return utils.Errorf("bind SSA project database failed: update hash: %s", err)
	}
	if err := OpenSSAProjectDatabase(project); err != nil {
		return err
	}
	SetCurrentSSAProjectID(profileDB, uint64(project.ID))
	return nil
}

func isSSADatabasePathActive(path string) bool {
	if path == "" {
		return false
	}
	active := consts.GetActiveSSADatabaseRawPath()
	absPath, err1 := filepath.Abs(path)
	absActive, err2 := filepath.Abs(active)
	if err1 != nil || err2 != nil {
		return path == active
	}
	return absPath == absActive
}

// closeSSADatabaseBeforeFileOp closes cached and active SSA handles so sqlite files can be removed on Windows.
func closeSSADatabaseBeforeFileOp(path string) (wasActive bool, err error) {
	wasActive = isSSADatabasePathActive(path)
	if err := consts.CloseSSADBPath(path); err != nil {
		return wasActive, err
	}
	return wasActive, consts.CloseGormSSAProjectDatabase()
}

func switchToDefaultSSADatabase(profileDB *gorm.DB) error {
	if err := OpenSSAProjectDatabaseRaw(ResolveDefaultSSADatabasePath()); err != nil {
		return err
	}
	clearCurrentSSAProjectID(profileDB)
	return nil
}

func initDedicatedSSAProjectDatabaseFile(path string) error {
	if path == "" {
		return utils.Errorf("init SSA project database file failed: path is empty")
	}
	tempDB, err := consts.CreateSSAProjectDatabaseRaw(path)
	if err != nil {
		return utils.Errorf("init SSA project database file failed: %s", err)
	}
	return tempDB.Close()
}

func reconnectDedicatedSSAProjectDatabase(profileDB *gorm.DB, project *schema.SSAProject) error {
	if project == nil || project.DatabasePath == "" {
		return utils.Errorf("reconnect SSA project database failed: dedicated path is empty")
	}
	if err := OpenSSAProjectDatabase(project); err != nil {
		return utils.Errorf("reconnect SSA project database failed: %s", err)
	}
	SetCurrentSSAProjectID(profileDB, uint64(project.ID))
	return nil
}

// removeDedicatedSSAProjectDatabaseFile closes the connection, deletes the sqlite file, and optionally reopens default.
func removeDedicatedSSAProjectDatabaseFile(profileDB *gorm.DB, project *schema.SSAProject, reopenDefault bool) error {
	if project == nil {
		return utils.Errorf("remove SSA project database failed: project is nil")
	}
	dbPath := ResolveSSAProjectDatabasePath(project)
	if IsDefaultSSADatabasePath(dbPath) {
		return nil
	}

	wasActive, err := closeSSADatabaseBeforeFileOp(dbPath)
	if err != nil {
		return utils.Errorf("close SSA database before delete failed: %s", err)
	}
	if err := consts.DeleteDatabaseFile(dbPath); err != nil {
		return utils.Errorf("delete SSA database file failed: %s", err)
	}
	if reopenDefault && (wasActive || !consts.IsGormSSAProjectDatabaseOpen()) {
		return switchToDefaultSSADatabase(profileDB)
	}
	return nil
}

// resetDedicatedSSAProjectDatabase clears compile data by replacing the project's sqlite file.
func resetDedicatedSSAProjectDatabase(profileDB *gorm.DB, project *schema.SSAProject) error {
	if project == nil {
		return utils.Errorf("reset SSA project database failed: project is nil")
	}
	if err := OpenSSAProjectDatabase(project); err != nil {
		return utils.Errorf("open SSA database before reset failed: %s", err)
	}
	dbPath := ResolveSSAProjectDatabasePath(project)
	if IsDefaultSSADatabasePath(dbPath) {
		_, err := DeleteSSAProgram(consts.GetGormSSAProjectDataBase(), &ypb.SSAProgramFilter{
			ProjectIds: []uint64{uint64(project.ID)},
		})
		return err
	}

	wasActive, err := closeSSADatabaseBeforeFileOp(dbPath)
	if err != nil {
		return utils.Errorf("close SSA database before reset failed: %s", err)
	}
	if err := consts.DeleteDatabaseFile(dbPath); err != nil {
		return utils.Errorf("delete SSA database file failed: %s", err)
	}
	if err := initDedicatedSSAProjectDatabaseFile(project.DatabasePath); err != nil {
		return err
	}
	if wasActive || GetCurrentSSAProjectID() == uint64(project.ID) {
		return reconnectDedicatedSSAProjectDatabase(profileDB, project)
	}
	return nil
}

func deleteSSAProjectRecord(profileDB *gorm.DB, project *schema.SSAProject) error {
	if profileDB == nil || project == nil || project.ID == 0 {
		return utils.Errorf("delete SSA project record failed: invalid args")
	}
	result := profileDB.Model(&schema.SSAProject{}).Where("id = ?", project.ID).Unscoped().Delete(&schema.SSAProject{})
	if result.Error != nil {
		return result.Error
	}
	if GetCurrentSSAProjectID() == uint64(project.ID) {
		return switchToDefaultSSADatabase(profileDB)
	}
	return nil
}
