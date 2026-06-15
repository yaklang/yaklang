//go:build darwin

package browser

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// brandedICNS 是 Chrome 图标 + 右下角 yak 公牛角标的合成图标。
// 仅在 macOS 构建中嵌入, 用于替换 rod 自动下载的 Chromium 默认图标,
// 让 AI 打开的浏览器在 Dock 中可被识别为 yak 的浏览器。
//
//go:embed assets/chrome_yak.icns
var brandedICNS []byte

// brandMarker 版本化标记: 若以后更新图标, 提升版本号即可触发重新打标。
const brandMarker = ".yak_branded_v4"

// applyBrandedIcon 将受 rod 管理的 Chromium.app 图标替换为品牌图标。
// 设计约束:
//   - 仅处理 rod 自己下载的 Chromium(由调用方保证 managed), 绝不修改用户自定义的系统浏览器
//   - 幂等: 通过 marker 文件避免重复写入
//   - rod 重新下载新版本 Chromium 后, 新 bundle 无 marker 会自动重新打标
//   - 任何失败都不影响浏览器启动(仅打日志)
//
// 关键词: 浏览器图标, chrome icon, yak 角标, app.icns 替换
func applyBrandedIcon(binPath string) {
	defer func() {
		if e := recover(); e != nil {
			log.Warnf("apply branded browser icon panic: %v", e)
		}
	}()

	if binPath == "" || len(brandedICNS) == 0 {
		return
	}

	// binPath 形如 .../Chromium.app/Contents/MacOS/Chromium
	macOSDir := filepath.Dir(binPath)      // .../Contents/MacOS
	contentsDir := filepath.Dir(macOSDir)  // .../Contents
	appBundle := filepath.Dir(contentsDir) // .../Chromium.app
	if !strings.HasSuffix(appBundle, ".app") {
		return
	}

	resourcesDir := filepath.Join(contentsDir, "Resources")
	if info, err := os.Stat(resourcesDir); err != nil || !info.IsDir() {
		return
	}

	marker := filepath.Join(resourcesDir, brandMarker)
	if _, err := os.Stat(marker); err == nil {
		return // 已打标, 跳过
	}

	// Chromium bundle 的 CFBundleIconFile 为 app.icns, 仅在其存在时覆盖
	iconPath := filepath.Join(resourcesDir, "app.icns")
	if _, err := os.Stat(iconPath); err != nil {
		return
	}

	if err := os.WriteFile(iconPath, brandedICNS, 0o644); err != nil {
		log.Warnf("write branded browser icon to %s failed: %v", iconPath, err)
		return
	}
	_ = os.WriteFile(marker, []byte(time.Now().Format(time.RFC3339)), 0o644)

	// 触碰 bundle 修改时间, 促使 macOS 刷新图标缓存
	now := time.Now()
	_ = os.Chtimes(appBundle, now, now)
	log.Infof("branded chromium app icon applied at %s", iconPath)
}
