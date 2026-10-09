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

// YakProject 描述引擎识别的项目数据库及其可用性、大小与最近操作日期。
// DatabaseID 区分不同 Yakit/Memfit 登记库中相同的数字 ProjectID，供 db.projectID 查询使用。
type YakProject struct {
	DatabaseID, Name, Description, DatabasePath, Source, Type string
	ProjectID                                                 uint
	SizeBytes                                                 int64
	LastOperationAt                                           time.Time
	Current, Available, SupportsHTTP                          bool
}

// Dump 把项目元信息格式化为可直接 println 的文本。
// SizeBytes 包括数据库主文件和 WAL；LastOperationAt 取项目登记更新时间与数据库文件修改时间中较新者，不代表本次读取时间。
//
// 返回值:
//   - text: 数据库标识、名称、来源、类型、当前/可用标记、HTTP 查询能力、大小、最近操作日期、路径与描述
//
// Example:
// ```
// for project in db.ListYakProjects()~ { println(project.Dump()) }
// ```
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

// ListYakProjects 列出引擎、Yakit 和 Memfit 登记的 Yaklang 项目（导出名为 db.ListYakProjects）。
//
// 默认读取当前运行绑定的项目登记库，并从 YAKIT_HOME、MEMFITAI_HOME、用户目录和两个应用的 config.json 发现其他登记库。
// 只读取登记信息，不切换当前项目、不修复登记记录。相同数据库路径去重；不同登记库可以复用数字 ProjectID，跨库查询请使用返回的 DatabaseID。
// 当前项目优先，其余按 LastOperationAt 降序排列；keyword 不区分大小写地匹配项目名称和描述，默认 limit=10、offset=0。
//
// 参数:
//   - opts: 项目列表选项（可变参数），支持 db.keyword、db.limit、db.offset；limit 范围 1–100，offset 范围 0–1000000
//
// 返回值:
//   - projects: YakProject 列表；包含 DatabaseID、ProjectID、Name、Description、DatabasePath、Source、Type、Current、Available、SupportsHTTP、SizeBytes、LastOperationAt；无匹配返回空列表
//   - err: 登记库读取失败、选项无效或上下文取消时返回错误
//
// SizeBytes 为 SQLite 主文件与 WAL 文件大小之和；LastOperationAt 取登记更新时间与数据库/WAL 修改时间的较新值，不代表最后读取时间。
// 只有 Available 和 SupportsHTTP 都为 true 的项目可用于 HTTP 查询；未登记的当前数据库以 current 标识展示，内存数据库大小为 0。
//
// <|EXAMPLE_START|> 列出项目并直接输出文本
// ```
// projects, err = db.ListYakProjects(db.limit(10), db.offset(0))
// if err != nil { die(err) }
// for project in projects { println(project.Dump()) }
// ```
// <|EXAMPLE_END|>
//
// <|EXAMPLE_START|> 按名称或描述查找项目
// ```
// projects = db.ListYakProjects(db.keyword("认证调查"), db.limit(5))~
//
//	for project in projects {
//	    println(project.Name, project.SizeBytes, project.LastOperationAt)
//	}
//
// ```
// <|EXAMPLE_END|>
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
