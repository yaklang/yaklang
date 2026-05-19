package yakit

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
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

// OpenSSAProjectDatabaseRaw switches the global SSA IR database connection.
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

// OpenSSAProjectByID loads the project from profile DB and switches the SSA connection.
func OpenSSAProjectByID(profileDB *gorm.DB, id uint64) (*schema.SSAProject, error) {
	if id == 0 {
		return nil, utils.Errorf("open SSA project failed: id is required")
	}
	project, err := GetSSAProjectById(id)
	if err != nil {
		if isSSAProjectNotFound(err) {
			clearCurrentSSAProjectIDIfMatch(id)
			return nil, utils.Errorf("open SSA project failed: project %d not found", id)
		}
		return nil, err
	}
	if err := OpenSSAProjectDatabase(project); err != nil {
		return nil, utils.Errorf("open SSA project database failed: %s", err)
	}
	SetCurrentSSAProjectID(profileDB, id)
	return project, nil
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

func ensureDefaultSSADatabaseOpen() error {
	if consts.IsGormSSAProjectDatabaseOpen() &&
		isSSADatabasePathActive(ResolveDefaultSSADatabasePath()) {
		return nil
	}
	return OpenSSAProjectDatabaseRaw(ResolveDefaultSSADatabasePath())
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

// closeSSADatabaseIfActive closes the global SSA connection when it points at path.
func closeSSADatabaseIfActive(path string) (wasActive bool, err error) {
	wasActive = isSSADatabasePathActive(path)
	if !wasActive {
		return false, nil
	}
	return true, consts.CloseGormSSAProjectDatabase()
}

// closeSSADatabaseBeforeFileOp closes the global SSA handle so sqlite files can be removed on Windows.
func closeSSADatabaseBeforeFileOp(path string) (wasActive bool, err error) {
	wasActive = isSSADatabasePathActive(path)
	// Always close the global handle: path comparison can fail across switches, but the file may still be locked.
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
	if err := consts.SetGormSSAProjectDatabaseByInfo(project.DatabasePath); err != nil {
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
