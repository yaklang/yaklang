package yaklib

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// YakProject identifies a registered engine database, independently of the app's
// numeric project ID (different Yakit/Memfit profiles may reuse that number).
type YakProject struct {
	DatabaseID, Name, Description, DatabasePath, Source, Type string
	ProjectID                                                 uint
	SizeBytes                                                 int64
	LastOperationAt                                           time.Time
	Current, Available, SupportsHTTP                          bool
}

func (p *YakProject) Dump() string {
	return fmt.Sprintf("database_id=%s name=%q source=%s type=%s current=%t available=%t supports_http=%t\nsize=%s size_bytes=%d last_operation_at=%s\ndatabase_path=%s\ndescription=%s", p.DatabaseID, p.Name, p.Source, p.Type, p.Current, p.Available, p.SupportsHTTP, utils.ByteSize(uint64(p.SizeBytes)), p.SizeBytes, p.LastOperationAt.Format(time.RFC3339), p.DatabasePath, p.Description)
}
func databaseFilePath(db *gorm.DB) string {
	if db == nil {
		return ""
	}
	rows, err := db.DB().Query("PRAGMA database_list")
	if err != nil {
		return ""
	}
	defer rows.Close()
	for rows.Next() {
		var seq int
		var name, path string
		if rows.Scan(&seq, &name, &path) == nil && name == "main" {
			return path
		}
	}
	return ""
}
func canonicalProjectPath(path string) string {
	if path == "" {
		return ""
	}
	path, _ = filepath.Abs(path)
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	return filepath.Clean(path)
}
func openHistoryReadOnly(path string) (*gorm.DB, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("database is not a regular file")
	}
	uriPath := filepath.ToSlash(canonicalProjectPath(path))
	// SQLite file URIs need /C:/... for Windows drive paths.
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	uri := (&url.URL{Scheme: "file", Path: uriPath}).String()
	db, err := gorm.Open("sqlite3", uri+"?mode=ro&_query_only=1&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.DB().SetMaxOpenConns(1)
	db.DB().SetMaxIdleConns(1)
	return db, nil
}
func knownProjectHomes() map[string]string {
	home := utils.GetHomeDirDefault(".")
	homes := map[string]string{}
	add := func(path, source string) {
		if path == "" {
			return
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(home, path)
		}
		path = canonicalProjectPath(path)
		homes[path] = mergeProjectSources(homes[path], source)
	}
	add(consts.GetDefaultYakitBaseDir(), "engine")
	add(filepath.Join(home, "yakit-projects"), "yakit")
	add(os.Getenv("YAKIT_HOME"), "yakit")
	add(os.Getenv("MEMFITAI_HOME"), "memfit")
	for _, app := range []string{"yakit", "memfit"} {
		f, err := os.Open(filepath.Join(home, ".yakit", app, "config.json"))
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
		_ = f.Close()
		if err != nil || len(data) > 1<<20 {
			continue
		}
		var config struct {
			Home string `json:"YAKIT_HOME"`
		}
		if json.Unmarshal(data, &config) == nil {
			add(config.Home, app)
		}
	}
	return homes
}
func allYakProjects(c *dbHistoryConfig) ([]*YakProject, error) {
	if c.projectDB == nil {
		c.projectDB = consts.GetGormProjectDatabase()
	}
	if c.profileDB == nil {
		c.profileDB = consts.GetGormProfileDatabase()
	}
	currentPath := canonicalProjectPath(databaseFilePath(c.projectDB))
	projects := map[string]*YakProject{}
	profiles := map[string]bool{}
	readProfile := func(db *gorm.DB, profilePath, base, source string) error {
		if !db.HasTable(&schema.Project{}) {
			return nil
		}
		var rows []*schema.Project
		if err := db.Model(&schema.Project{}).Where("type IS NULL OR type != ?", "file").Order("id ASC").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if err := c.ctx.Err(); err != nil {
				return err
			}
			if row.DatabasePath == "" || strings.Contains(row.DatabasePath, "://") {
				continue
			}
			path := row.DatabasePath
			if !filepath.IsAbs(path) {
				path = filepath.Join(base, path)
			}
			if _, err := os.Stat(path); os.IsNotExist(err) {
				folder := "projects"
				if row.Type == "ssa_project" {
					folder = "ssa-projects"
				}
				moved := filepath.Join(base, folder, filepath.Base(path))
				if _, err := os.Stat(moved); err == nil {
					path = moved
				}
			}
			path = canonicalProjectPath(path)
			if previous := projects[path]; previous != nil {
				previous.Source = mergeProjectSources(previous.Source, source)
				continue
			}
			key := sha256.Sum256([]byte(profilePath + "\x00" + fmt.Sprint(row.ID) + "\x00" + path))
			typ := row.Type
			if typ == "" {
				typ = "project"
			}
			p := &YakProject{DatabaseID: "yak-" + hex.EncodeToString(key[:12]), ProjectID: row.ID, Name: row.ProjectName, Description: row.Description, DatabasePath: path, Source: source, Type: typ, LastOperationAt: row.UpdatedAt, Current: path != "" && path == currentPath, SupportsHTTP: typ == "project"}
			updateProjectFileInfo(p)
			projects[path] = p
		}
		return nil
	}
	profilePath := canonicalProjectPath(databaseFilePath(c.profileDB))
	if c.profileDB != nil {
		if err := readProfile(c.profileDB, profilePath, filepath.Dir(profilePath), "engine"); err != nil {
			return nil, err
		}
		profiles[profilePath] = true
	}
	homes := knownProjectHomes()
	paths := make([]string, 0, len(homes))
	for path := range homes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, base := range paths {
		path := canonicalProjectPath(filepath.Join(base, consts.YAK_PROFILE_PLUGIN_DB_NAME))
		if profiles[path] {
			for _, p := range projects {
				if strings.HasPrefix(p.DatabasePath, base+string(filepath.Separator)) {
					p.Source = mergeProjectSources(p.Source, homes[base])
				}
			}
			continue
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		db, err := openHistoryReadOnly(path)
		if err != nil {
			return nil, fmt.Errorf("read project profile %s: %w", path, err)
		}
		err = readProfile(db, path, base, homes[base])
		_ = db.Close()
		if err != nil {
			return nil, err
		}
		profiles[path] = true
	}
	if c.projectDB != nil && projects[currentPath] == nil {
		p := &YakProject{DatabaseID: "current", Name: "[current]", DatabasePath: currentPath, Source: "engine", Type: "project", Current: true, SupportsHTTP: true, Available: true}
		updateProjectFileInfo(p)
		if currentPath == "" {
			p.Available = true
		}
		projects[currentPath] = p
	}
	result := make([]*YakProject, 0, len(projects))
	for _, p := range projects {
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Current != result[j].Current {
			return result[i].Current
		}
		if !result[i].LastOperationAt.Equal(result[j].LastOperationAt) {
			return result[i].LastOperationAt.After(result[j].LastOperationAt)
		}
		return result[i].DatabaseID < result[j].DatabaseID
	})
	return result, c.ctx.Err()
}
func updateProjectFileInfo(p *YakProject) {
	for _, suffix := range []string{"", "-wal"} {
		if info, err := os.Stat(p.DatabasePath + suffix); err == nil && info.Mode().IsRegular() {
			p.SizeBytes += info.Size()
			if info.ModTime().After(p.LastOperationAt) {
				p.LastOperationAt = info.ModTime()
			}
			if suffix == "" {
				p.Available = true
			}
		}
	}
}

// ListYakProjects reads engine and installed Yakit/Memfit project registries.
// It does not switch the active DB or repair/migrate any profile records.
func ListYakProjects(opts ...DBHistoryOption) ([]*YakProject, error) {
	c, err := historyConfig(2048, opts)
	if err != nil {
		return nil, err
	}
	all, err := allYakProjects(c)
	if err != nil {
		return nil, err
	}
	selected := []*YakProject{}
	for _, p := range all {
		if c.keyword == "" || strings.Contains(strings.ToLower(p.Name+" "+p.Description), strings.ToLower(c.keyword)) {
			selected = append(selected, p)
		}
	}
	start := c.offset
	if start > len(selected) {
		start = len(selected)
	}
	end := start + c.limit
	if end > len(selected) {
		end = len(selected)
	}
	return selected[start:end], nil
}
func historyProjectDatabase(c *dbHistoryConfig) (*gorm.DB, func(), error) {
	if c.projectID == "" || c.projectID == "current" {
		if c.projectDB == nil {
			c.projectDB = consts.GetGormProjectDatabase()
		}
		if c.projectDB == nil {
			return nil, nil, fmt.Errorf("current project database is unavailable")
		}
		return c.projectDB, func() {}, nil
	}
	projects, err := allYakProjects(c)
	if err != nil {
		return nil, nil, err
	}
	for _, p := range projects {
		if p.DatabaseID != c.projectID {
			continue
		}
		if !p.Available || !p.SupportsHTTP {
			return nil, nil, fmt.Errorf("project %s is unavailable or not an HTTP project", c.projectID)
		}
		if p.Current {
			return c.projectDB, func() {}, nil
		}
		db, err := openHistoryReadOnly(p.DatabasePath)
		if err != nil {
			return nil, nil, err
		}
		return db, func() { _ = db.Close() }, nil
	}
	return nil, nil, fmt.Errorf("unknown database_id; use db.ListYakProjects first")
}

func mergeProjectSources(a, b string) string {
	sources := map[string]bool{}
	for _, source := range strings.Split(a+","+b, ",") {
		if source != "" {
			sources[source] = true
		}
	}
	result := make([]string, 0, len(sources))
	for source := range sources {
		result = append(result, source)
	}
	sort.Strings(result)
	return strings.Join(result, ",")
}
