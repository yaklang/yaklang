package aiforge

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// ForgeExportOption customizes export behavior.
type ForgeExportOption func(*forgeExportOptions)

type forgeExportOptions struct {
	progress func(percent float64, message string)
	password string
	output   string
}

// WithExportProgress registers a progress callback (percent 0-100) for export.
func WithExportProgress(cb func(percent float64, message string)) ForgeExportOption {
	return func(o *forgeExportOptions) {
		o.progress = cb
	}
}

// WithExportPassword sets password to encrypt the export zip (SM4-CBC).
func WithExportPassword(password string) ForgeExportOption {
	return func(o *forgeExportOptions) {
		o.password = password
	}
}

// WithExportOutputName sets the output zip base name (without extension).
func WithExportOutputName(name string) ForgeExportOption {
	return func(o *forgeExportOptions) {
		o.output = name
	}
}

func applyExportOptions(opts ...ForgeExportOption) *forgeExportOptions {
	cfg := &forgeExportOptions{}
	for _, o := range opts {
		if o != nil {
			o(cfg)
		}
	}
	return cfg
}

// ForgeImportOption customizes import behavior.
type ForgeImportOption func(*forgeImportOptions)

type forgeImportOptions struct {
	progress  func(percent float64, message string)
	overwrite bool
	newName   string
	password  string
}

// WithImportProgress registers a progress callback (percent 0-100) for import.
func WithImportProgress(cb func(percent float64, message string)) ForgeImportOption {
	return func(o *forgeImportOptions) {
		o.progress = cb
	}
}

// WithImportOverwrite controls whether existing forge is overwritten.
func WithImportOverwrite(overwrite bool) ForgeImportOption {
	return func(o *forgeImportOptions) {
		o.overwrite = overwrite
	}
}

// WithImportNewName overrides the forge name when a single forge is imported.
func WithImportNewName(name string) ForgeImportOption {
	return func(o *forgeImportOptions) {
		o.newName = name
	}
}

// WithImportPassword sets password to decrypt the import zip (SM4-CBC).
func WithImportPassword(password string) ForgeImportOption {
	return func(o *forgeImportOptions) {
		o.password = password
	}
}

func applyImportOptions(opts ...ForgeImportOption) *forgeImportOptions {
	cfg := &forgeImportOptions{}
	for _, o := range opts {
		if o != nil {
			o(cfg)
		}
	}
	return cfg
}

// ExportAIForgesToZip exports one or more forges (and optional AI tools) into a zip package.
// Each forge will be placed in its own directory under the archive.
// For config/json type, the package layout follows buildinforges (e.g. buildinforge/hostscan).
// For yak type, only forge_cfg.json and <name>.yak are included.
func ExportAIForgesToZip(db *gorm.DB, forgeNames []string, toolNames []string, targetPath string, opts ...ForgeExportOption) (string, error) {
	if db == nil {
		return "", utils.Error("db is required")
	}
	if len(forgeNames) == 0 && len(toolNames) == 0 {
		return "", utils.Error("forge names or tool names are required")
	}
	opt := applyExportOptions(opts...)

	tmpDir, err := os.MkdirTemp("", "aiforge-export-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	progress := func(percent float64, msg string) {
		if opt.progress != nil {
			opt.progress(percent, msg)
		}
	}
	progress(0, "start export")

	total := len(forgeNames) + len(toolNames)
	progressStep := func(done int, name string) {
		if total == 0 {
			return
		}
		progress(float64(done)/float64(total)*100, name)
	}

	exported := 0
	for _, name := range forgeNames {
		if strings.TrimSpace(name) == "" {
			return "", utils.Error("empty forge name detected")
		}
		forge, err := yakit.GetAIForgeByName(db, name)
		if err != nil {
			return "", err
		}
		effectiveName := forge.ForgeName
		forgeDir := filepath.Join(tmpDir, effectiveName)
		if err := dumpForgeToDir(forge, forgeDir, effectiveName); err != nil {
			return "", err
		}
		exported++
		progressStep(exported, fmt.Sprintf("exported forge %s", effectiveName))
	}

	for _, name := range toolNames {
		if strings.TrimSpace(name) == "" {
			return "", utils.Error("empty tool name detected")
		}
		tool, err := yakit.GetAIYakTool(db, name)
		if err != nil {
			return "", err
		}
		toolDir := filepath.Join(tmpDir, "tools", tool.Name)
		if err := dumpAIToolToDir(tool, toolDir); err != nil {
			return "", err
		}
		exported++
		progressStep(exported, fmt.Sprintf("exported tool %s", tool.Name))
	}

	if opt.output == "" {
		if len(forgeNames) == 1 && len(toolNames) == 0 {
			opt.output = forgeNames[0]
		} else {
			opt.output = "aiforge-package"
		}
	}
	finalName := opt.output + ".zip"
	if opt.password != "" {
		finalName += ".enc"
	}
	if targetPath == "" {
		targetPath = filepath.Join(consts.GetDefaultYakitProjectsDir(), finalName)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", err
	}

	zipBytes, err := createZipFromDir(tmpDir, tmpDir)
	if err != nil {
		return "", err
	}
	if opt.password != "" {
		zipBytes, err = codec.SM4EncryptCBCWithPKCSPadding(
			codec.PKCS7Padding([]byte(opt.password)),
			zipBytes,
			codec.PKCS7Padding([]byte(opt.password)),
		)
		if err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(targetPath, zipBytes, 0o644); err != nil {
		return "", err
	}
	progress(100, "export completed")
	return targetPath, nil
}

// ImportAIForgesFromZip imports one or more forges (and optional AI tools) from a zip package.
func ImportAIForgesFromZip(db *gorm.DB, zipPath string, opts ...ForgeImportOption) ([]*schema.AIForge, error) {
	if db == nil {
		return nil, utils.Error("db is required")
	}
	if zipPath == "" {
		return nil, utils.Error("zip path is required")
	}
	if exist, _ := utils.PathExists(zipPath); !exist {
		return nil, utils.Errorf("zip path not exists: %s", zipPath)
	}
	opt := applyImportOptions(opts...)

	tmpDir, err := os.MkdirTemp("", "aiforge-import-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	progress := func(percent float64, msg string) {
		if opt.progress != nil {
			opt.progress(percent, msg)
		}
	}
	progress(0, "start import")

	fileBytes, err := os.ReadFile(zipPath)
	if err != nil {
		return nil, err
	}
	if opt.password != "" {
		fileBytes, err = codec.SM4DecryptCBCWithPKCSPadding(
			codec.PKCS7Padding([]byte(opt.password)),
			fileBytes,
			codec.PKCS7Padding([]byte(opt.password)),
		)
		if err != nil {
			return nil, err
		}
	}
	if err := extractZipBytes(fileBytes, tmpDir); err != nil {
		return nil, err
	}

	cfgPaths, err := findAllForgeCfg(tmpDir)
	if err != nil {
		return nil, err
	}
	toolCfgPaths, err := findAllAIToolCfg(tmpDir)
	if err != nil {
		return nil, err
	}
	if len(cfgPaths) == 0 && len(toolCfgPaths) == 0 {
		return nil, utils.Error("neither forge_cfg.json nor tool_cfg.json found in package")
	}

	total := len(cfgPaths) + len(toolCfgPaths)
	current := 0
	progressStep := func(msg string) {
		current++
		progress(float64(current)/float64(total)*100, msg)
	}

	var forges []*schema.AIForge
	for _, p := range cfgPaths {
		effectiveName := ""
		if opt.newName != "" && len(cfgPaths) == 1 {
			effectiveName = opt.newName
		}
		forge, err := loadForgeFromDir(db, filepath.Dir(p), effectiveName, opt)
		if err != nil {
			return nil, err
		}
		forges = append(forges, forge)
		progressStep(fmt.Sprintf("imported forge %s", forge.ForgeName))
	}

	for _, p := range toolCfgPaths {
		tool, err := loadAIToolFromDir(db, filepath.Dir(p), opt)
		if err != nil {
			return nil, err
		}
		progressStep(fmt.Sprintf("imported tool %s", tool.Name))
	}

	progress(100, "import completed")
	return forges, nil
}

func detectForgeType(forge *schema.AIForge) string {
	forgeType := normalizeForgeType(forge.ForgeType)
	if forgeType != "" {
		return forgeType
	}
	if forge.InitPrompt != "" || forge.PersistentPrompt != "" || forge.PlanPrompt != "" || forge.ResultPrompt != "" {
		return schema.FORGE_TYPE_Config
	}
	return schema.FORGE_TYPE_YAK
}

func normalizeForgeType(raw string) string {
	ft := strings.ToLower(strings.TrimSpace(raw))
	switch ft {
	case "":
		return ""
	case "json", schema.FORGE_TYPE_Config:
		return schema.FORGE_TYPE_Config
	case schema.FORGE_TYPE_YAK:
		return schema.FORGE_TYPE_YAK
	default:
		return ft
	}
}

func writePromptFiles(dir string, forge *schema.AIForge) error {
	prompts := map[string]string{
		"init.txt":       forge.InitPrompt,
		"persistent.txt": forge.PersistentPrompt,
		"plan.txt":       forge.PlanPrompt,
		"result.txt":     forge.ResultPrompt,
	}
	for filename, content := range prompts {
		if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func findAllForgeCfg(root string) ([]string, error) {
	var res []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if info.Name() == "forge_cfg.json" {
			res = append(res, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

type aiToolPackageConfig struct {
	Name              string `json:"name,omitempty"`
	VerboseName       string `json:"verbose_name,omitempty"`
	Description       string `json:"description,omitempty"`
	Keywords          string `json:"keywords,omitempty"`
	Params            string `json:"params,omitempty"`
	Path              string `json:"path,omitempty"`
	EnableAIOutputLog int    `json:"enable_ai_output_log,omitempty"`
}

func findAllAIToolCfg(root string) ([]string, error) {
	var res []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if info.Name() == "tool_cfg.json" {
			res = append(res, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func loadPromptFiles(dir string) (string, string, string, string) {
	return readFileIfExists(filepath.Join(dir, "init.txt")),
		readFileIfExists(filepath.Join(dir, "persistent.txt")),
		readFileIfExists(filepath.Join(dir, "plan.txt")),
		readFileIfExists(filepath.Join(dir, "result.txt"))
}

func readYakContent(dir, forgeName string) (string, error) {
	defaultYak := filepath.Join(dir, fmt.Sprintf("%s.yak", forgeName))
	if data, err := os.ReadFile(defaultYak); err == nil {
		return string(data), nil
	}
	var yakPath string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if filepath.Ext(info.Name()) == ".yak" {
			yakPath = path
			return io.EOF
		}
		return nil
	})
	if yakPath == "" {
		return "", utils.Error("yak file not found in package")
	}
	data, err := os.ReadFile(yakPath)
	return string(data), err
}

func readFileIfExists(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func dumpForgeToDir(forge *schema.AIForge, forgeDir string, effectiveName string) error {
	forgeType := detectForgeType(forge)

	if err := os.MkdirAll(forgeDir, 0o755); err != nil {
		return err
	}

	cfg := NewYakForgeBlueprintConfigFromSchemaForge(forge)
	cfg.ForgeType = forgeType
	if cfg.Author == "" {
		cfg.Author = forge.Author
	}
	if effectiveName != "" {
		cfg.Name = effectiveName
	}
	cfgBytes, err := json.MarshalIndent(cfg, "", "\t")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(forgeDir, "forge_cfg.json"), cfgBytes, 0o644); err != nil {
		return err
	}

	yakName := forge.ForgeName
	if effectiveName != "" {
		yakName = effectiveName
	}
	if err := os.WriteFile(filepath.Join(forgeDir, fmt.Sprintf("%s.yak", yakName)), []byte(forge.ForgeContent), 0o644); err != nil {
		return err
	}

	if forgeType == schema.FORGE_TYPE_Config {
		if err := writePromptFiles(forgeDir, forge); err != nil {
			return err
		}
	}
	return nil
}

func dumpAIToolToDir(tool *schema.AIYakTool, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cfg := aiToolPackageConfig{
		Name:              tool.Name,
		VerboseName:       tool.VerboseName,
		Description:       tool.Description,
		Keywords:          tool.Keywords,
		Params:            tool.Params,
		Path:              tool.Path,
		EnableAIOutputLog: tool.EnableAIOutputLog,
	}
	cfgBytes, err := json.MarshalIndent(cfg, "", "\t")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "tool_cfg.json"), cfgBytes, 0o644); err != nil {
		return err
	}

	yakName := tool.Name
	if yakName == "" {
		yakName = filepath.Base(dir)
	}
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s.yak", yakName)), []byte(tool.Content), 0o644); err != nil {
		return err
	}
	return nil
}

func loadForgeFromDir(db *gorm.DB, cfgDir string, overrideName string, opt *forgeImportOptions) (*schema.AIForge, error) {
	cfgPath := filepath.Join(cfgDir, "forge_cfg.json")
	cfgBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}
	var cfg YakForgeBlueprintConfig
	if err := json.Unmarshal(cfgBytes, &cfg); err != nil {
		return nil, err
	}

	forgeName := cfg.Name
	if forgeName == "" {
		forgeName = filepath.Base(cfgDir)
	}
	if overrideName != "" {
		forgeName = overrideName
	}

	forgeType := normalizeForgeType(cfg.ForgeType)
	initPrompt, persistentPrompt, planPrompt, resultPrompt := loadPromptFiles(cfgDir)
	if initPrompt == "" {
		initPrompt = cfg.InitPrompt
	}
	if persistentPrompt == "" {
		persistentPrompt = cfg.PersistentPrompt
	}
	if planPrompt == "" {
		planPrompt = cfg.PlanPrompt
	}
	if resultPrompt == "" {
		resultPrompt = cfg.ResultPrompt
	}

	yakContent, yakErr := readYakContent(cfgDir, forgeName)
	if yakErr != nil && cfg.ForgeContent == "" {
		return nil, yakErr
	}

	if forgeType == "" {
		if initPrompt != "" || persistentPrompt != "" || planPrompt != "" || resultPrompt != "" {
			forgeType = schema.FORGE_TYPE_Config
		} else {
			forgeType = schema.FORGE_TYPE_YAK
		}
	}

	forge := &schema.AIForge{
		ForgeName:          forgeName,
		ForgeVerboseName:   cfg.VerboseName,
		ForgeContent:       yakContent,
		ForgeType:          forgeType,
		Author:             cfg.Author,
		ParamsUIConfig:     cfg.ParamsUIConfig,
		Params:             cfg.CLIParameterRuleYaklangCode,
		UserPersistentData: cfg.UserPersistentData,
		Description:        cfg.Description,
		Tools:              cfg.Tools,
		ToolKeywords:       cfg.ToolKeywords,
		Actions:            cfg.Actions,
		Tags:               cfg.Tags,
		InitPrompt:         initPrompt,
		PersistentPrompt:   persistentPrompt,
		PlanPrompt:         planPrompt,
		ResultPrompt:       resultPrompt,
	}
	if forge.ForgeContent == "" {
		forge.ForgeContent = cfg.ForgeContent
	}

	if opt != nil {
		if !opt.overwrite {
			if _, err := yakit.GetAIForgeByName(db, forge.ForgeName); err == nil {
				return nil, utils.Errorf("forge %s already exists", forge.ForgeName)
			}
		}
	}

	if err := yakit.CreateOrUpdateAIForgeByName(db, forge.ForgeName, forge); err != nil {
		return nil, err
	}
	return forge, nil
}

func loadAIToolFromDir(db *gorm.DB, cfgDir string, opt *forgeImportOptions) (*schema.AIYakTool, error) {
	cfgPath := filepath.Join(cfgDir, "tool_cfg.json")
	cfgBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}
	var cfg aiToolPackageConfig
	if err := json.Unmarshal(cfgBytes, &cfg); err != nil {
		return nil, err
	}
	toolName := cfg.Name
	if toolName == "" {
		toolName = filepath.Base(cfgDir)
	}
	content, err := readYakContent(cfgDir, toolName)
	if err != nil {
		return nil, err
	}
	tool := &schema.AIYakTool{
		Name:              toolName,
		VerboseName:       cfg.VerboseName,
		Description:       cfg.Description,
		Keywords:          cfg.Keywords,
		Content:           content,
		Params:            cfg.Params,
		Path:              cfg.Path,
		EnableAIOutputLog: cfg.EnableAIOutputLog,
	}
	if tool.EnableAIOutputLog == 0 {
		tool.EnableAIOutputLog = yakscripttools.ParseAIToolEnableAIOutputLog(tool.Content)
	}

	if opt != nil && !opt.overwrite {
		if _, err := yakit.GetAIYakTool(db, tool.Name); err == nil {
			return nil, utils.Errorf("tool %s already exists", tool.Name)
		}
	}

	if existing, err := yakit.GetAIYakTool(db, tool.Name); err == nil {
		tool.ID = existing.ID
		tool.CreatedAt = existing.CreatedAt
		if _, err := yakit.UpdateAIYakToolByID(db, tool); err != nil {
			return nil, err
		}
		return tool, nil
	}

	if _, err := yakit.CreateAIYakTool(db, tool); err != nil {
		return nil, err
	}
	return tool, nil
}

func createZipFromDir(srcDir, baseRoot string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(baseRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			header.Name += "/"
		}
		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
	if err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func extractZipBytes(data []byte, dst string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range r.File {
		target := filepath.Join(dst, f.Name)
		if !strings.HasPrefix(target, filepath.Clean(dst)+string(os.PathSeparator)) {
			return utils.Errorf("invalid path in zip: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
