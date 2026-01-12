package yaklib

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/yaklang/yaklang/common/mimetype"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	defaultFileScanMaxSize = 10 * 1024 * 1024
	defaultRiskType        = "file_malicious"
)

type FileScanConfig struct {
	IncludePatterns  []string              `json:"include_patterns"`
	ExcludePatterns  []string              `json:"exclude_patterns"`
	Recursive        bool                  `json:"recursive"`
	MaxFileSize      int64                 `json:"max_file_size"`
	EnableRisk       bool                  `json:"enable_risk"`
	RiskType         string                `json:"risk_type"`
	RiskTitle        string                `json:"risk_title"`
	RiskTitleVerbose string                `json:"risk_title_verbose"`
	RiskDescription  string                `json:"risk_description"`
	CustomSignatures []interface{}         `json:"-"`
	ResultCallback   func(*FileScanResult) `json:"-"`
	AlertCallback    func(*FileScanResult) `json:"-"`
}

type FileScanResult struct {
	Path         string                `json:"path"`
	Name         string                `json:"name"`
	IsDir        bool                  `json:"is_dir"`
	Size         int64                 `json:"size"`
	Mode         string                `json:"mode"`
	ModTime      int64                 `json:"mod_time"`
	Extension    string                `json:"extension"`
	MimeType     string                `json:"mime_type"`
	Md5          string                `json:"md5,omitempty"`
	Sha1         string                `json:"sha1,omitempty"`
	Sha256       string                `json:"sha256,omitempty"`
	Matched      bool                  `json:"matched"`
	Matches      []*MaliciousSignature `json:"matches,omitempty"`
	MatchNames   []string              `json:"match_names,omitempty"`
	RiskID       int64                 `json:"risk_id,omitempty"`
	RiskSeverity string                `json:"risk_severity,omitempty"`
	Skipped      bool                  `json:"skipped"`
	SkipReason   string                `json:"skip_reason,omitempty"`
	Error        string                `json:"error,omitempty"`
}

type FileScanner struct {
	config  *FileScanConfig
	matcher *MaliciousFileMatcher
}

func NewFileScanner(config *FileScanConfig) (*FileScanner, error) {
	if config == nil {
		config = &FileScanConfig{}
	}
	if config.MaxFileSize == 0 {
		config.MaxFileSize = defaultFileScanMaxSize
	}
	if config.RiskType == "" {
		config.RiskType = defaultRiskType
	}
	if config.RiskTitleVerbose == "" {
		config.RiskTitleVerbose = config.RiskTitle
	}

	matcher := NewMaliciousFileMatcher()
	for _, sig := range config.CustomSignatures {
		if err := matcher.AddSignature(sig); err != nil {
			return nil, err
		}
	}

	return &FileScanner{
		config:  config,
		matcher: matcher,
	}, nil
}

func (fs *FileScanner) SetResultCallback(callback func(*FileScanResult)) {
	fs.config.ResultCallback = callback
}

func (fs *FileScanner) SetAlertCallback(callback func(*FileScanResult)) {
	fs.config.AlertCallback = callback
}

func (fs *FileScanner) GetConfig() *FileScanConfig {
	return fs.config
}

func (fs *FileScanner) ScanFile(path string) (*FileScanResult, error) {
	result, err := fs.scanFile(path)
	if result == nil {
		return result, err
	}
	processErr := fs.handleResult(result)
	if processErr != nil {
		err = utils.JoinErrors(err, processErr)
		if result.Error == "" {
			result.Error = processErr.Error()
		}
	}
	return result, err
}

func (fs *FileScanner) ScanDir(path string) ([]*FileScanResult, error) {
	if path == "" {
		return nil, utils.Errorf("path cannot be empty")
	}

	var results []*FileScanResult
	var scanErr error

	if fs.config.Recursive {
		err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
			if err != nil {
				scanErr = utils.JoinErrors(scanErr, err)
				return nil
			}
			if info == nil || info.IsDir() {
				return nil
			}
			if !fs.shouldScan(filePath) {
				return nil
			}
			result, err := fs.ScanFile(filePath)
			if result != nil {
				results = append(results, result)
			}
			if err != nil {
				scanErr = utils.JoinErrors(scanErr, err)
			}
			return nil
		})
		if err != nil {
			scanErr = utils.JoinErrors(scanErr, err)
		}
		return results, scanErr
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filePath := filepath.Join(path, entry.Name())
		if !fs.shouldScan(filePath) {
			continue
		}
		result, err := fs.ScanFile(filePath)
		if result != nil {
			results = append(results, result)
		}
		if err != nil {
			scanErr = utils.JoinErrors(scanErr, err)
		}
	}
	return results, scanErr
}

func (fs *FileScanner) scanFile(path string) (*FileScanResult, error) {
	if path == "" {
		return nil, utils.Errorf("file path cannot be empty")
	}
	if !fs.shouldScan(path) {
		return &FileScanResult{
			Path:       path,
			Name:       filepath.Base(path),
			Skipped:    true,
			SkipReason: "excluded",
		}, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return &FileScanResult{
			Path:  path,
			Name:  filepath.Base(path),
			Error: err.Error(),
		}, err
	}

	result := &FileScanResult{
		Path:      path,
		Name:      info.Name(),
		IsDir:     info.IsDir(),
		Size:      info.Size(),
		Mode:      info.Mode().String(),
		ModTime:   info.ModTime().Unix(),
		Extension: filepath.Ext(path),
	}

	if info.IsDir() {
		result.Skipped = true
		result.SkipReason = "is_dir"
		return result, utils.Errorf("path is directory: %s", path)
	}

	result.MimeType = detectFileMimeType(path, result.Extension)

	if fs.config.MaxFileSize > 0 && info.Size() > fs.config.MaxFileSize {
		result.Skipped = true
		result.SkipReason = "file_too_large"
		return result, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	result.Md5 = hashMd5(content)
	result.Sha1 = hashSha1(content)
	result.Sha256 = hashSha256(content)

	if fs.matcher != nil {
		matches := fs.matcher.MatchContentWithDetails(content)
		if len(matches) > 0 {
			result.Matched = true
			result.Matches = matches
			result.MatchNames = signatureNames(matches)
			result.RiskSeverity = strongestSeverity(matches)
		}
	}

	return result, nil
}

func (fs *FileScanner) handleResult(result *FileScanResult) error {
	if fs.config.ResultCallback != nil {
		fs.config.ResultCallback(result)
	}
	if !result.Matched || !fs.config.EnableRisk {
		return nil
	}
	_, err := fs.createRisk(result)
	if err != nil {
		return err
	}
	if fs.config.AlertCallback != nil {
		fs.config.AlertCallback(result)
	}
	return nil
}

func (fs *FileScanner) createRisk(result *FileScanResult) (*schema.Risk, error) {
	riskType := fs.config.RiskType
	if riskType == "" {
		riskType = defaultRiskType
	}

	title := fs.config.RiskTitle
	if title == "" {
		title = fmt.Sprintf("Malicious file detected: %s", filepath.Base(result.Path))
	}

	titleVerbose := fs.config.RiskTitleVerbose
	if titleVerbose == "" {
		titleVerbose = title
	}

	description := fs.config.RiskDescription
	if description == "" {
		description = fmt.Sprintf("File matched %d malicious signatures", len(result.Matches))
	}

	details := buildRiskDetails(result)
	severity := result.RiskSeverity
	if severity == "" {
		severity = strongestSeverity(result.Matches)
	}

	target := buildFileTarget(result.Path)
	risk, err := yakit.NewRisk(
		target,
		yakit.WithRiskParam_Title(title),
		yakit.WithRiskParam_TitleVerbose(titleVerbose),
		yakit.WithRiskParam_RiskType(riskType),
		yakit.WithRiskParam_Severity(severity),
		yakit.WithRiskParam_Description(description),
		yakit.WithRiskParam_Details(details),
		yakit.WithRiskParam_Payload(result.Path),
	)
	if err != nil {
		return nil, err
	}
	if risk != nil {
		result.RiskID = int64(risk.ID)
	}
	return risk, nil
}

func (fs *FileScanner) shouldScan(path string) bool {
	if len(fs.config.IncludePatterns) > 0 {
		matched := false
		for _, pattern := range fs.config.IncludePatterns {
			matched, _ = filepath.Match(pattern, filepath.Base(path))
			if !matched {
				re, err := regexp.Compile(pattern)
				if err == nil && re.MatchString(path) {
					matched = true
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return false
		}
	}

	for _, pattern := range fs.config.ExcludePatterns {
		matched, _ := filepath.Match(pattern, filepath.Base(path))
		if !matched {
			re, err := regexp.Compile(pattern)
			if err == nil && re.MatchString(path) {
				matched = true
			}
		}
		if matched {
			return false
		}
	}

	return true
}

func buildFileTarget(path string) string {
	if strings.HasPrefix(path, "file://") {
		return path
	}
	if !filepath.IsAbs(path) {
		return path
	}
	normalized := filepath.ToSlash(path)
	if runtime.GOOS == "windows" {
		return "file:///" + normalized
	}
	return "file://" + normalized
}

func detectFileMimeType(path, ext string) string {
	mimeType, err := mimetype.DetectFile(path)
	if err == nil && mimeType != nil {
		if mimeType.String() != "application/octet-stream" {
			return mimeType.String()
		}
	}
	if ext == "" {
		ext = filepath.Ext(path)
	}
	return _getTypeByExtension(ext)
}

func buildRiskDetails(result *FileScanResult) string {
	details := map[string]interface{}{
		"path":        result.Path,
		"size":        result.Size,
		"mime_type":   result.MimeType,
		"md5":         result.Md5,
		"sha1":        result.Sha1,
		"sha256":      result.Sha256,
		"match_names": result.MatchNames,
		"matches":     result.Matches,
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return fmt.Sprintf("path=%s; matches=%v", result.Path, result.MatchNames)
	}
	return string(raw)
}

func signatureNames(matches []*MaliciousSignature) []string {
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		if match == nil {
			continue
		}
		if match.Name == "" {
			continue
		}
		names = append(names, match.Name)
	}
	return names
}

func strongestSeverity(matches []*MaliciousSignature) string {
	levels := map[string]int{
		"low":      1,
		"medium":   2,
		"high":     3,
		"critical": 4,
	}
	strongest := ""
	highest := 0
	for _, match := range matches {
		if match == nil {
			continue
		}
		level := strings.ToLower(match.Severity)
		if level == "" {
			level = "low"
		}
		rank := levels[level]
		if rank == 0 {
			level = "low"
			rank = levels[level]
		}
		if rank > highest {
			highest = rank
			strongest = level
		}
	}
	if strongest == "" {
		return "low"
	}
	return strongest
}

func hashMd5(content []byte) string {
	sum := md5.Sum(content)
	return hex.EncodeToString(sum[:])
}

func hashSha1(content []byte) string {
	sum := sha1.Sum(content)
	return hex.EncodeToString(sum[:])
}

func hashSha256(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func parseFileScannerConfigFromMap(config map[string]interface{}) *FileScanConfig {
	scanConfig := &FileScanConfig{
		Recursive:   true,
		MaxFileSize: defaultFileScanMaxSize,
		EnableRisk:  true,
		RiskType:    defaultRiskType,
	}

	if includes := parseStringSliceFromConfig(config, "include_patterns"); includes != nil {
		scanConfig.IncludePatterns = includes
	}
	if excludes := parseStringSliceFromConfig(config, "exclude_patterns"); excludes != nil {
		scanConfig.ExcludePatterns = excludes
	}
	if recursive, ok := config["recursive"]; ok {
		scanConfig.Recursive = utils.InterfaceToBoolean(recursive)
	}
	if maxSize, ok := config["max_file_size"]; ok {
		scanConfig.MaxFileSize = int64(utils.InterfaceToInt(maxSize))
	}
	if enableRisk, ok := config["enable_risk"]; ok {
		scanConfig.EnableRisk = utils.InterfaceToBoolean(enableRisk)
	}
	if riskType, ok := config["risk_type"]; ok {
		scanConfig.RiskType = utils.InterfaceToString(riskType)
	}
	if riskTitle, ok := config["risk_title"]; ok {
		scanConfig.RiskTitle = utils.InterfaceToString(riskTitle)
	}
	if riskTitleVerbose, ok := config["risk_title_verbose"]; ok {
		scanConfig.RiskTitleVerbose = utils.InterfaceToString(riskTitleVerbose)
	}
	if riskDesc, ok := config["risk_description"]; ok {
		scanConfig.RiskDescription = utils.InterfaceToString(riskDesc)
	}
	if sigs, ok := config["custom_signatures"]; ok {
		scanConfig.CustomSignatures = utils.InterfaceToSliceInterface(sigs)
	}

	return scanConfig
}

var FileScannerExports = map[string]interface{}{
	"NewScanner": func(config map[string]interface{}) (*FileScanner, error) {
		return NewFileScanner(parseFileScannerConfigFromMap(config))
	},
}
