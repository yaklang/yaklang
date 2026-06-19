package browser

import (
	"fmt"
	"sync"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/yaklang/yaklang/common/log"
)

var globalManager = &BrowserManager{
	browsers: make(map[string]*BrowserInstance),
}

// HaveBrowserInstalled 检测当前环境是否已安装可用的浏览器（导出名为 browser.HaveBrowserInstalled）
// 参数:
//   - 无
//
// 返回值:
//   - 是否已安装浏览器
//
// Example:
// ```
// has = browser.HaveBrowserInstalled()
// println(has)
// ```
func HaveBrowserInstalled() bool {
	_, has := launcher.LookPath()
	return has
}

type BrowserManager struct {
	mu       sync.RWMutex
	browsers map[string]*BrowserInstance
}

// Open 打开一个新的浏览器实例（导出名为 browser.Open）
// 参数:
//   - opts: 浏览器可选项，如 browser.headless、browser.proxy、browser.exePath 等
//
// 返回值:
//   - 浏览器实例对象
//   - 错误信息
//
// Example:
// ```
// // 打开一个无头浏览器（示意性示例，需要本地已安装浏览器）
// b = browser.Open(browser.headless(true))~
// defer browser.CloseAll()
// ```
func Open(opts ...BrowserOption) (*BrowserInstance, error) {
	return globalManager.Open(opts...)
}

// Get 获取一个已存在的浏览器实例（不存在时按选项创建，导出名为 browser.Get）
// 参数:
//   - opts: 浏览器可选项，如 browser.id 用于按 ID 获取
//
// 返回值:
//   - 浏览器实例对象
//   - 错误信息
//
// Example:
// ```
// // 按 ID 获取浏览器实例（示意性示例）
// b = browser.Get(browser.id("main"))~
// dump(b)
// ```
func Get(opts ...BrowserOption) (*BrowserInstance, error) {
	return globalManager.Get(opts...)
}

// List 列出当前所有已打开浏览器实例的 ID（导出名为 browser.List）
// 参数:
//   - 无
//
// 返回值:
//   - 浏览器实例 ID 列表
//
// Example:
// ```
// ids = browser.List()
// dump(ids)
// ```
func List() []string {
	return globalManager.List()
}

// CloseByID 关闭指定 ID 的浏览器实例（导出名为 browser.Close）
// 参数:
//   - opts: 浏览器可选项，通常使用 browser.id 指定要关闭的实例
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 关闭指定 ID 的浏览器（示意性示例）
// err = browser.Close(browser.id("main"))
// if err != nil { die(err) }
// ```
func CloseByID(opts ...BrowserOption) error {
	return globalManager.CloseByID(opts...)
}

// CloseAll 关闭当前所有已打开的浏览器实例（导出名为 browser.CloseAll）
// 参数:
//   - 无
//
// 返回值:
//   - 无
//
// Example:
// ```
// browser.CloseAll()
// ```
func CloseAll() {
	globalManager.CloseAll()
}

func (m *BrowserManager) Open(opts ...BrowserOption) (*BrowserInstance, error) {
	config := parseBrowserOptions(opts...)
	id := config.id

	m.mu.Lock()
	defer m.mu.Unlock()

	if inst, ok := m.browsers[id]; ok && !inst.IsClosed() {
		if err := inst.healthCheck(); err != nil {
			log.Warnf("browser instance %q unhealthy (%v), recreating", id, err)
			delete(m.browsers, id)
			_ = inst.Close()
		} else {
			log.Infof("reusing existing browser instance %q", id)
			return inst, nil
		}
	}

	inst, err := newBrowserInstance(id, config)
	if err != nil {
		return nil, fmt.Errorf("open browser %q: %w", id, err)
	}

	m.browsers[id] = inst
	return inst, nil
}

func (m *BrowserManager) Get(opts ...BrowserOption) (*BrowserInstance, error) {
	config := parseBrowserOptions(opts...)
	id := config.id

	m.mu.RLock()
	defer m.mu.RUnlock()

	inst, ok := m.browsers[id]
	if !ok {
		return nil, fmt.Errorf("browser instance %q not found, call Open() first", id)
	}
	if inst.IsClosed() {
		return nil, fmt.Errorf("browser instance %q is closed, call Open() to create a new one", id)
	}
	return inst, nil
}

func (m *BrowserManager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var ids []string
	for id, inst := range m.browsers {
		if !inst.IsClosed() {
			ids = append(ids, id)
		}
	}
	return ids
}

func (m *BrowserManager) CloseByID(opts ...BrowserOption) error {
	config := parseBrowserOptions(opts...)
	id := config.id

	m.mu.Lock()
	defer m.mu.Unlock()

	inst, ok := m.browsers[id]
	if !ok {
		return fmt.Errorf("browser instance %q not found", id)
	}

	err := inst.Close()
	delete(m.browsers, id)
	return err
}

// Evict removes a browser instance from the manager and closes it.
// Used when CDP is dead but IsClosed() is still false (zombie instance).
func (m *BrowserManager) Evict(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inst, ok := m.browsers[id]
	if !ok {
		return
	}
	delete(m.browsers, id)
	log.Warnf("evicting broken browser instance %q from manager", id)
	_ = inst.Close()
}

func (m *BrowserManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, inst := range m.browsers {
		if err := inst.Close(); err != nil {
			log.Errorf("close browser instance %q: %v", id, err)
		}
		delete(m.browsers, id)
	}
}
