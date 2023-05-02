package guard

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"
	"yaklang.io/yaklang/common/utils"
)

type Guard struct {
	// map[string]*PathGuardTarget
	paths *sync.Map
	// map[string]*PsAuxProcessGuardTarget
	procs *sync.Map
	// map[string]*NetConnGuardTarget
	conns *sync.Map
	// map[string]*NginxGuardTarget
	nginxes *sync.Map
	// map[string]*ApacheGuardTarget
	apaches *sync.Map
}

func NewGuard() *Guard {
	return &Guard{
		paths: new(sync.Map), procs: new(sync.Map), conns: new(sync.Map),
		nginxes: new(sync.Map), apaches: new(sync.Map),
	}
}

func (g *Guard) AddPathGuardWithRecover(
	id, path string, recursive bool, intervalSeconds int,
	options ...PathGuardTargetOption,
) error {
	preset := []PathGuardTargetOption{
		SetPathGuardCacheFileSize(4 * 1024 * 1024),
		SetPathGuardContentChangeCallback(func(old *GuardFileInfo, new *GuardFileInfo) {

		}),
	}
	options = append(preset, options...)
	options = append(options, SetPathUnserRecovered(true))
	return g.AddPathGuard(id, path, recursive, intervalSeconds, options...)
}

func (g *Guard) AddPathGuard(id, path string, recursive bool, intervalSeconds int, options ...PathGuardTargetOption) error {
	if intervalSeconds < 5 {
		return utils.Errorf("path watch guard interval should be larger than 5s")
	}

	var err error
	if !filepath.IsAbs(path) {
		path, err = filepath.Abs(path)
		if err != nil {
			return utils.Errorf("calc path %v to abs path failed: %s", path, err)
		}
	}

	var p = &PathGuardTarget{
		guardTargetBase: guardTargetBase{
			intervalSeconds: intervalSeconds,
		},
		Path:             path,
		Recursive:        recursive,
		cache:            new(sync.Map),
		recordOriginOnce: new(sync.Once),
		origin:           new(sync.Map),
		isFirst:          utils.NewAtomicBool(),
		disallowNewFile:  utils.NewAtomicBool(),
	}
	p.isFirst.Set()
	p.children = p
	for _, opt := range options {
		err := opt(p)
		if err != nil {
			return err
		}
	}
	g.paths.Store(id, p)
	return nil
}

func (g *Guard) AddProcessGuard(id string, intervalSeconds int, options ...PsAuxProcessGuardOption) error {
	t, err := NewPsAuxProcessGuardTarget(intervalSeconds, options...)
	if err != nil {
		return err
	}
	g.procs.Store(id, t)
	return nil
}

func (g *Guard) AddNetConnGuard(id string, intervalSeconds int, options ...NetConnGuardOption) error {
	t, err := NewNetConnGuardTarget(intervalSeconds, options...)
	if err != nil {
		return err
	}
	g.conns.Store(id, t)
	return nil
}

func (g *Guard) AddNginxGuard(id string, intervalSeconds int, cbs ...NginxGuardCallback) {
	t := NewNginxGuardTarget(intervalSeconds, cbs...)
	g.nginxes.Store(id, t)
}

func (g *Guard) AddApacheGuard(id string, intervalSeconds int, cbs ...ApacheGuardCallback) {
	t := NewApacheGuardTarget(intervalSeconds, cbs...)
	g.apaches.Store(id, t)
}

func (g *Guard) RemoveApacheGuard(id string) {
	g.apaches.Delete(id)
}

func (g *Guard) RemoveNginxGuard(id string) {
	g.nginxes.Delete(id)
}

func (g *Guard) RemovePathGuard(id string) {
	g.paths.Delete(id)
}

func (g *Guard) RemoveProcessGuard(id string) {
	g.procs.Delete(id)
}

func (g *Guard) RemoveNetConnGuard(id string) {
	g.conns.Delete(id)
}

func (g *Guard) Run(ctx context.Context) {
	tick1s := time.NewTicker(1 * time.Second)
	defer tick1s.Stop()

	for {
		select {
		case <-tick1s.C:
			for _, targets := range []*sync.Map{
				g.paths, g.procs, g.conns, g.nginxes, g.apaches,
			} {
				targets.Range(func(key, value interface{}) bool {
					sub, ok := value.(guardTargetInterface)
					if !ok {
						return true
					}
					sub.Do()
					sub.Next()
					return true
				})
			}
		case <-ctx.Done():
			return
		}
	}
}

func (g *Guard) TriggerProcessGuardActionById(id string) error {
	raw, ok := g.procs.Load(id)
	if !ok {
		return utils.Errorf("no such process guard for [%s]", id)
	}

	t := raw.(*PsAuxProcessGuardTarget)
	t.do()
	return nil
}

func (g *Guard) TriggerPathMonitorGuardActionById(id string) error {
	raw, ok := g.paths.Load(id)
	if !ok {
		return utils.Errorf("no such process guard for [%s]", id)
	}

	t := raw.(*PathGuardTarget)
	t.do()
	return nil
}

func (g *Guard) TriggerNetConnectionGuardActionById(id string) error {
	raw, ok := g.conns.Load(id)
	if !ok {
		return utils.Errorf("no such process guard for [%s]", id)
	}

	t := raw.(*NetConnGuardTarget)
	t.do()
	return nil
}

func (g *Guard) RecoverFile(p string) error {
	var rawContent []byte
	g.paths.Range(func(key, value interface{}) bool {
		v := value.(*PathGuardTarget)
		raw, ok := v.origin.Load(p)
		if !ok {
			return true
		}

		file := raw.(*GuardFileInfo)
		rawContent = file.Content
		return true
	})

	if rawContent == nil {
		return utils.Errorf("path: %s not existed or not under monitoring...", p)
	}
	_ = os.Remove(p)
	return ioutil.WriteFile(p, rawContent, os.ModePerm)
}

func (g *Guard) GetOriginFileContent(p string) ([]byte, error) {
	var rawContent []byte
	g.paths.Range(func(key, value interface{}) bool {
		v := value.(*PathGuardTarget)
		raw, ok := v.origin.Load(p)
		if !ok {
			return true
		}

		file := raw.(*GuardFileInfo)
		rawContent = file.Content
		return true
	})
	if rawContent != nil {
		return rawContent, nil
	}
	return nil, utils.Errorf("empty file or not under monitored/recovered")
}

func (g *Guard) GetAllFilesUnderRecovered() []string {
	var path []string
	g.paths.Range(func(key, value interface{}) bool {
		v := value.(*PathGuardTarget)
		v.origin.Range(func(key, value interface{}) bool {
			path = append(path, key.(string))
			return true
		})
		return true
	})
	return path
}

type guardTargetInterface interface {
	Do()
	do()
	Next()
}

type guardTargetBase struct {
	guardTargetInterface

	intervalSeconds int
	intervalOffset  int
	children        guardTargetInterface
}

func (g *guardTargetBase) Do() {
	if g.intervalOffset == 0 {
		g.children.do()
	}
}

func (g *guardTargetBase) Next() {
	g.intervalOffset = (g.intervalOffset + 1) % g.intervalSeconds
}
