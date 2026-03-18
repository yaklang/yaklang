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

func HaveBrowserInstalled() bool {
	_, has := launcher.LookPath()
	return has
}

type BrowserManager struct {
	mu       sync.RWMutex
	browsers map[string]*BrowserInstance
}

func Open(opts ...BrowserOption) (*BrowserInstance, error) {
	return globalManager.Open(opts...)
}

func Get(opts ...BrowserOption) (*BrowserInstance, error) {
	return globalManager.Get(opts...)
}

func List() []string {
	return globalManager.List()
}

func CloseByID(opts ...BrowserOption) error {
	return globalManager.CloseByID(opts...)
}

func CloseAll() {
	globalManager.CloseAll()
}

func (m *BrowserManager) Open(opts ...BrowserOption) (*BrowserInstance, error) {
	config := parseBrowserOptions(opts...)
	id := config.id

	m.mu.Lock()
	defer m.mu.Unlock()

	if inst, ok := m.browsers[id]; ok && !inst.IsClosed() {
		log.Infof("reusing existing browser instance %q", id)
		return inst, nil
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
