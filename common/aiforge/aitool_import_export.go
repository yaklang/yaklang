package aiforge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func ExportAIToolsToZip(ctx context.Context, db *gorm.DB, filter *ypb.AIToolFilter, targetPath string, opts ...ForgeExportOption) (string, error) {
	if db == nil {
		return "", utils.Error("db is required")
	}

	opt := applyExportOptions(opts...)

	tmpDir, err := os.MkdirTemp("", "aitool-export-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	progress := func(percent float64, msg string, messageType string) {
		if opt.progress != nil {
			opt.progress(percent, msg, messageType)
		}
	}
	progress(0, "start export", "info")

	total, err := yakit.CountAIYakTools(db, filter)
	if err != nil {
		return "", utils.Wrapf(err, "count tools failed")
	}

	progressStep := func(done int, name string, messageType string) {
		if total == 0 {
			return
		}
		progress(float64(done)/float64(total)*100, name, messageType)
	}

	exported := 0
	progressErrorMsg := func(msg string) {
		progressStep(exported, "[Error]: "+msg, "error")
	}
	resolvedToolNames := make([]string, 0)

	for tool := range yakit.YieldAIYakTools(ctx, db, filter) {
		if tool == nil {
			log.Errorf("empty tool detected")
			progressErrorMsg("empty tool detected")
			continue
		}
		effectiveName := tool.Name
		resolvedToolNames = append(resolvedToolNames, effectiveName)
		toolDir := filepath.Join(tmpDir, effectiveName)
		if err := dumpAIToolToDir(tool, toolDir); err != nil {
			log.Errorf("failed to export tool %s: %v", effectiveName, err)
			progressErrorMsg(fmt.Sprintf("failed to export tool %s: %v", effectiveName, err))
			continue
		}
		exported++
		progressStep(exported, fmt.Sprintf("exported tool %s", effectiveName), "info")
	}

	if opt.output == "" {
		if total == 1 && len(resolvedToolNames) == 1 {
			opt.output = resolvedToolNames[0]
		} else {
			opt.output = "aitool-package"
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
	progress(100, "export completed", "success")
	return targetPath, nil
}

func LoadAIToolsFromZip(zipPath string, opts ...ForgeImportOption) ([]*schema.AIYakTool, error) {
	opt := applyImportOptions(opts...)

	archiveInfo, err := LoadAIForgesFromZip(zipPath, opts...)
	if err != nil {
		return nil, err
	}
	if len(archiveInfo.AIYakTools) == 0 {
		return nil, utils.Error("no AI tools found in package")
	}
	if opt.newToolName != "" && len(archiveInfo.AIYakTools) == 1 {
		archiveInfo.AIYakTools[0].Name = opt.newToolName
	}
	return archiveInfo.AIYakTools, nil
}

func ImportAIToolsFromZip(db *gorm.DB, zipPath string, opts ...ForgeImportOption) ([]*schema.AIYakTool, error) {
	if db == nil {
		return nil, utils.Error("db is required")
	}

	opt := applyImportOptions(opts...)

	originalProgress := opt.progress
	loadProgress := func(percent float64, msg string) {
		if originalProgress != nil {
			originalProgress(percent*0.5, msg)
		}
	}

	loadOpts := make([]ForgeImportOption, 0, len(opts)+1)
	loadOpts = append(loadOpts, opts...)
	loadOpts = append(loadOpts, WithImportProgress(loadProgress))

	tools, err := LoadAIToolsFromZip(zipPath, loadOpts...)
	if err != nil {
		return nil, err
	}

	progress := func(percent float64, msg string) {
		if originalProgress != nil {
			originalProgress(50+percent*0.5, msg)
		}
	}
	progress(0, "start importing to database")

	total := len(tools)
	current := 0
	progressStep := func(msg string) {
		current++
		progress(float64(current)/float64(total)*100, msg)
	}

	importedTools := make([]*schema.AIYakTool, 0, len(tools))
	for _, tool := range tools {
		if !opt.overwrite {
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
		} else {
			if _, err := yakit.CreateAIYakTool(db, tool); err != nil {
				return nil, err
			}
		}
		importedTools = append(importedTools, tool)
		progressStep(fmt.Sprintf("imported tool %s", tool.Name))
	}

	progress(100, "import completed")
	return importedTools, nil
}
