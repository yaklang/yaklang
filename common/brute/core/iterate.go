package core

import (
	"context"
	"sync/atomic"
)

// Combination 是惰性生成的一个 目标 × 用户名 × 密码 组合。
type Combination struct {
	Target   string
	Username string
	Password string
	Index    int64
}

// CombinationSource 惰性产出组合序列。
// Next 在序列耗尽时返回 false；实现必须是无状态或幂等安全。
type CombinationSource interface {
	Next(ctx context.Context) (Combination, bool)
}

// cartesianSource 按 目标 × 密码 × 用户 的顺序惰性生成完整笛卡尔积，
// 不预生成任何切片 —— 内存复杂度 O(1)。
// 顺序与旧 mixer.NewMixer(target, pass, users) 一致以保持行为兼容。
type cartesianSource struct {
	targets, passwords, usernames []string
	ti, pi, ui                    int
	index                         int64
}

// NewCartesianSource 创建惰性笛卡尔积源。
func NewCartesianSource(targets, usernames, passwords []string) CombinationSource {
	return &cartesianSource{
		targets:   targets,
		passwords: passwords,
		usernames: usernames,
	}
}

func (c *cartesianSource) Next(ctx context.Context) (Combination, bool) {
	if err := ctx.Err(); err != nil {
		return Combination{}, false
	}
	if len(c.targets) == 0 || len(c.usernames) == 0 || len(c.passwords) == 0 {
		return Combination{}, false
	}
	if c.ti >= len(c.targets) {
		return Combination{}, false
	}
	comb := Combination{
		Target:   c.targets[c.ti],
		Password: c.passwords[c.pi],
		Username: c.usernames[c.ui],
		Index:    c.index,
	}
	c.index++
	// 逐层进位：user → password → target
	c.ui++
	if c.ui >= len(c.usernames) {
		c.ui = 0
		c.pi++
		if c.pi >= len(c.passwords) {
			c.pi = 0
			c.ti++
		}
	}
	return comb, true
}

// countingSource 包装另一个源并维护已产出计数（原子）。
type countingSource struct {
	inner CombinationSource
	count atomic.Int64
}

// NewCountingSource 包装组合源，返回源与计数器。
func NewCountingSource(inner CombinationSource) (CombinationSource, *atomic.Int64) {
	src := &countingSource{inner: inner}
	return src, &src.count
}

func (c *countingSource) Next(ctx context.Context) (Combination, bool) {
	comb, ok := c.inner.Next(ctx)
	if ok {
		c.count.Add(1)
	}
	return comb, ok
}

func (c *countingSource) Count() int64 { return c.count.Load() }

// sliceSource 把固定组合切片适配为源（测试与兼容入口用）。
type sliceSource struct {
	items []Combination
	pos   int
}

// NewSliceSource 基于既有切片创建源。切片本身已物化，仅用于兼容场景。
func NewSliceSource(items []Combination) CombinationSource {
	return &sliceSource{items: items}
}

func (s *sliceSource) Next(ctx context.Context) (Combination, bool) {
	if s.pos >= len(s.items) {
		return Combination{}, false
	}
	item := s.items[s.pos]
	s.pos++
	return item, true
}

// ErrStopSource 是哨兵：源已被目标级短路逻辑标记结束时复用。
type emptySource struct{}

// NewEmptySource 返回立即耗尽的源。
func NewEmptySource() CombinationSource { return emptySource{} }

func (emptySource) Next(context.Context) (Combination, bool) { return Combination{}, false }
