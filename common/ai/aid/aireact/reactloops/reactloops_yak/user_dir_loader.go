package reactloops_yak

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// 此文件实现「用户级 yak focus mode 自动发现」：
//   - 扫描 ~/yakit-projects/ai-focus/<name>/<name>.ai-focus.yak
//   - 同级 *.yak 作为 sidekick 自动加载
//   - 懒加载：只有调用 EnsureUserFocusModesLoaded 时才扫盘，
//     绝不在 init() 触发，避免拖慢 yak 启动
//   - 限频：内部冷却（默认 5s，可被 env YAK_AI_FOCUS_RELOAD_INTERVAL 覆盖）
//   - 加性：扫到的新 focus mode 注册到全局表；同名跳过；删除文件不会反注册
//   - 错误隔离：单个 focus mode 加载失败只 log.Warn，不影响其它
//
// 关键词: user-defined yak focus mode, yakit-projects/ai-focus, lazy load,
// cooldown, additive registration

const (
	// envUserFocusReloadInterval 允许通过环境变量覆盖默认 5s 冷却窗口。
	envUserFocusReloadInterval = "YAK_AI_FOCUS_RELOAD_INTERVAL"

	defaultUserFocusReloadInterval = 5 * time.Second
)

var (
	userFocusMu       sync.Mutex
	userFocusCooldown *utils.CoolDown
	userFocusLoaded   = make(map[string]struct{}) // 已注册成功的 focus mode name 集合

	// userFocusDirOverride 仅供测试覆盖默认目录路径，
	// 生产代码不应该写它。空字符串表示使用 consts.GetDefaultYakitAIFocusDir()。
	userFocusDirOverride string
)

// EnsureUserFocusModesLoaded 限频扫描 ~/yakit-projects/ai-focus/，把所有
// *.ai-focus.yak 注册到 reactloops 全局注册表。
//
// 行为约定：
//   - 多次调用幂等；冷却窗口内（默认 5 秒）的后续调用直接返回 nil，不扫盘
//   - 并发安全：互斥锁保证同时只有一次扫盘在跑
//   - 加性：已注册的 focus mode（同名）跳过，不报错
//   - 单 focus mode 失败只 log.Warn，不影响其它 focus mode 与主流程
//   - 删除磁盘文件不会反注册（重启清理）
//
// 注入点：grpc QueryAIFocus / StartAIReAct 入口；CLI 不需要调用（CLI 显式 --file）
//
// 关键词: ensure user yak focus modes loaded, lazy scan, cooldown gate
func EnsureUserFocusModesLoaded() error {
	userFocusMu.Lock()
	cd := ensureUserFocusCooldownLocked()
	userFocusMu.Unlock()

	var scanErr error
	cd.DoOr(func() {
		scanErr = scanAndRegisterUserFocusModes()
	}, func() {
		// 冷却中，直接放过
		scanErr = nil
	})
	return scanErr
}

// ensureUserFocusCooldownLocked 懒初始化 cooldown 实例。必须持有 userFocusMu 调用。
func ensureUserFocusCooldownLocked() *utils.CoolDown {
	if userFocusCooldown != nil {
		return userFocusCooldown
	}
	d := defaultUserFocusReloadInterval
	if env := strings.TrimSpace(os.Getenv(envUserFocusReloadInterval)); env != "" {
		if parsed, err := time.ParseDuration(env); err == nil && parsed > 0 {
			d = parsed
		} else {
			log.Warnf("user yak focus loader: invalid %s=%q, fallback to %s",
				envUserFocusReloadInterval, env, defaultUserFocusReloadInterval)
		}
	}
	userFocusCooldown = utils.NewCoolDown(d)
	return userFocusCooldown
}

// scanAndRegisterUserFocusModes 真正执行一次磁盘扫描 + 注册。
//
// 返回的 error 仅指向「无法读取根目录」这种致命错误；单个 focus mode 失败
// 不会冒到这里，只 log。
func scanAndRegisterUserFocusModes() error {
	root := userFocusRootDir()
	info, err := os.Stat(root)
	if err != nil {
		// 目录不存在视为空目录（首次安装时常见）。
		if os.IsNotExist(err) {
			log.Debugf("user yak focus loader: root dir not exist, skip: %s", root)
			return nil
		}
		return utils.Wrapf(err, "stat user yak focus dir %s", root)
	}
	if !info.IsDir() {
		log.Warnf("user yak focus loader: root path is not a dir, skip: %s", root)
		return nil
	}

	subEntries, err := os.ReadDir(root)
	if err != nil {
		return utils.Wrapf(err, "read user yak focus dir %s", root)
	}

	registered := 0
	skipped := 0
	failed := 0

	for _, sub := range subEntries {
		if !sub.IsDir() {
			continue
		}
		subDir := filepath.Join(root, sub.Name())
		entry, ok := findFocusEntryFile(subDir)
		if !ok {
			continue
		}
		entryPath := filepath.Join(subDir, entry)
		bundle, err := LoadSingleFile(entryPath)
		if err != nil {
			log.Warnf("user yak focus loader: load %s failed: %v", entryPath, err)
			failed++
			continue
		}

		if isAlreadyRegistered(bundle.Name) {
			skipped++
			continue
		}

		if err := reactloops.RegisterYakFocusModeFromBundle(bundle); err != nil {
			log.Warnf("user yak focus loader: register %s failed: %v", bundle.Name, err)
			failed++
			continue
		}

		userFocusMu.Lock()
		userFocusLoaded[bundle.Name] = struct{}{}
		userFocusMu.Unlock()
		registered++
		log.Infof("user yak focus loader: registered focus mode %q from %s",
			bundle.Name, entryPath)
	}

	if registered > 0 || failed > 0 {
		log.Infof("user yak focus loader: scan done root=%s registered=%d skipped=%d failed=%d",
			root, registered, skipped, failed)
	}
	return nil
}

// userFocusRootDir 返回扫描根目录。优先 testing override，其次 consts。
func userFocusRootDir() string {
	userFocusMu.Lock()
	override := userFocusDirOverride
	userFocusMu.Unlock()
	if override != "" {
		return override
	}
	return consts.GetDefaultYakitAIFocusDir()
}

// findFocusEntryFile 在目录下找第一个 *.ai-focus.yak 主入口文件名。
// 只取一个；若有多个，按字典序最小者作为主入口。
func findFocusEntryFile(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	var candidates []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, FocusModeFileSuffix) {
			candidates = append(candidates, name)
		}
	}
	if len(candidates) == 0 {
		return "", false
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}
	// 多个 *.ai-focus.yak 在同一目录，取字典序最小作为主入口；
	// 其余会被「主入口扫描器」识别为另一个 focus mode 的主入口（同级目录最多只有一个）。
	// 实际上同一目录建议只放一个主入口；这里保守取最小那个。
	first := candidates[0]
	for _, c := range candidates[1:] {
		if c < first {
			first = c
		}
	}
	log.Warnf("user yak focus loader: multiple ai-focus.yak in %s, picking %s",
		dir, first)
	return first, true
}

// isAlreadyRegistered 检查名字是否已在 reactloops 全局表 / 本 loader 已注册集合中。
func isAlreadyRegistered(name string) bool {
	if name == "" {
		return false
	}
	userFocusMu.Lock()
	_, ok := userFocusLoaded[name]
	userFocusMu.Unlock()
	if ok {
		return true
	}
	if _, exists := reactloops.GetLoopMetadata(name); exists {
		return true
	}
	return false
}

// SetUserFocusDirForTest 仅供测试覆盖默认根目录。
// 调用方负责在测试结束时用空字符串还原。
//
// 关键词: testing override user focus root dir
func SetUserFocusDirForTest(dir string) {
	userFocusMu.Lock()
	defer userFocusMu.Unlock()
	userFocusDirOverride = dir
}

// ResetUserFocusLoaderForTest 把 cooldown / loaded 集合 / override 全部重置。
// 仅供测试调用。
//
// 关键词: testing reset user focus loader state
func ResetUserFocusLoaderForTest() {
	userFocusMu.Lock()
	defer userFocusMu.Unlock()
	if userFocusCooldown != nil {
		userFocusCooldown.Close()
		userFocusCooldown = nil
	}
	userFocusLoaded = make(map[string]struct{})
	userFocusDirOverride = ""
}

// SnapshotUserFocusLoadedForTest 返回当前已注册的 focus mode name 副本，
// 仅供测试断言使用。
//
// 关键词: testing snapshot user focus loaded set
func SnapshotUserFocusLoadedForTest() map[string]struct{} {
	userFocusMu.Lock()
	defer userFocusMu.Unlock()
	out := make(map[string]struct{}, len(userFocusLoaded))
	for k, v := range userFocusLoaded {
		out[k] = v
	}
	return out
}
