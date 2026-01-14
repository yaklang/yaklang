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
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// ForgeTransferOption customizes import/export behavior.
type ForgeTransferOption func(*forgeTransferOptions)

type forgeTransferOptions struct {
	progress  func(percent float64, message string)
	overwrite bool
	newName   string
	author    string
	password  string
	output    string
}

// WithForgeProgress registers a progress callback (percent 0-100).
func WithForgeProgress(cb func(percent float64, message string)) ForgeTransferOption {
	return func(o *forgeTransferOptions) {
		o.progress = cb
	}
}

// WithForgeOverwrite controls whether existing forge (or output file) is overwritten.
func WithForgeOverwrite(overwrite bool) ForgeTransferOption {
	return func(o *forgeTransferOptions) {
		o.overwrite = overwrite
	}
}

// WithForgeNewName overrides the forge name when a single forge is imported/exported.
func WithForgeNewName(name string) ForgeTransferOption {
	return func(o *forgeTransferOptions) {
		o.newName = name
	}
}

// WithForgeAuthor overrides the author metadata when exporting/importing.
func WithForgeAuthor(author string) ForgeTransferOption {
	return func(o *forgeTransferOptions) {
		o.author = author
	}
}

// WithForgePassword sets password to encrypt/decrypt the export zip (SM4-CBC).
func WithForgePassword(password string) ForgeTransferOption {
	return func(o *forgeTransferOptions) {
		o.password = password
	}
}

// WithForgeOutputName sets the output zip base name (without extension).
func WithForgeOutputName(name string) ForgeTransferOption {
	return func(o *forgeTransferOptions) {
		o.output = name
	}
}

func applyTransferOptions(opts ...ForgeTransferOption) *forgeTransferOptions {
	cfg := &forgeTransferOptions{}
	for _, o := range opts {
		if o != nil {
			o(cfg)
		}
	}
	return cfg
}

// ExportAIForgesToTarGz exports one or more forges into a zip package.
// Each forge will be placed in its own directory under the archive.
// For config/json type, the package layout follows buildinforges (e.g. buildinforge/hostscan).
// For yak type, only forge_cfg.json and <name>.yak are included.
func ExportAIForgesToTarGz(db *gorm.DB, forgeNames []string, targetPath string, opts ...ForgeTransferOption) (string, error) {
	if db == nil {
		return "", utils.Error("db is required")
	}
	if len(forgeNames) == 0 {
		return "", utils.Error("forge names are required")
	}
	opt := applyTransferOptions(opts...)

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

	for idx, name := range forgeNames {
		if strings.TrimSpace(name) == "" {
			return "", utils.Error("empty forge name detected")
		}
		forge, err := yakit.GetAIForgeByName(db, name)
		if err != nil {
			return "", err
		}
		effectiveName := forge.ForgeName
		if opt.newName != "" && len(forgeNames) == 1 {
			effectiveName = opt.newName
		}
		if opt.author != "" {
			forge.Author = opt.author
		}
		forgeDir := filepath.Join(tmpDir, effectiveName)
		if err := dumpForgeToDir(forge, forgeDir, effectiveName); err != nil {
			return "", err
		}
		progress(float64(idx+1)/float64(len(forgeNames))*100, fmt.Sprintf("exported %s", effectiveName))
	}

	if opt.output == "" {
		if len(forgeNames) == 1 {
			opt.output = forgeNames[0]
		} else {
			opt.output = "aiforges"
		}
	}
	finalName := opt.output + ".zip"
	if opt.password != "" {
		finalName += ".enc"
	}
	if targetPath == "" {
		targetPath = filepath.Join(consts.GetDefaultYakitBaseTempDir(), finalName)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", err
	}

	if !opt.overwrite {
		if exist, _ := utils.PathExists(targetPath); exist {
			return "", utils.Errorf("target file already exists: %s", targetPath)
		}
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

// ImportAIForgesFromTarGz imports one or more forges from a tar.gz package.
func ImportAIForgesFromTarGz(db *gorm.DB, tarPath string, opts ...ForgeTransferOption) ([]*schema.AIForge, error) {
	if db == nil {
		return nil, utils.Error("db is required")
	}
	if tarPath == "" {
		return nil, utils.Error("tar.gz path is required")
	}
	if exist, _ := utils.PathExists(tarPath); !exist {
		return nil, utils.Errorf("tar.gz path not exists: %s", tarPath)
	}
	opt := applyTransferOptions(opts...)

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

	fileBytes, err := os.ReadFile(tarPath)
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
	var forges []*schema.AIForge
	for idx, p := range cfgPaths {
		effectiveName := ""
		if opt.newName != "" && len(cfgPaths) == 1 {
			effectiveName = opt.newName
		}
		forge, err := loadForgeFromDir(db, filepath.Dir(p), effectiveName, opt)
		if err != nil {
			return nil, err
		}
		forges = append(forges, forge)
		progress(float64(idx+1)/float64(len(cfgPaths))*100, fmt.Sprintf("imported %s", forge.ForgeName))
	}
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
	if len(res) == 0 {
		return nil, utils.Error("forge_cfg.json not found in package")
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

func loadForgeFromDir(db *gorm.DB, cfgDir string, overrideName string, opt *forgeTransferOptions) (*schema.AIForge, error) {
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
		if opt.author != "" {
			forge.Author = opt.author
		}
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
