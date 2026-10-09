package aimemory

import (
	"context"
	"strings"

	"github.com/yaklang/gorm"
)

type options struct {
	namespace, mode string
	limit, tokens   int
	ctx             context.Context
	db              *gorm.DB
}

type Option func(*options)

func WithMemoryNamespace(id string) Option {
	return func(c *options) {
		if namespace := strings.TrimSpace(id); namespace != "" {
			c.namespace = namespace
		}
	}
}
func WithMemoryTokenLimit(n int) Option { return func(c *options) { c.tokens = n } }
func WithMemoryLimit(n int) Option      { return func(c *options) { c.limit = n } }
func WithMemorySearchMode(mode string) Option {
	return func(c *options) { c.mode = strings.ToLower(strings.TrimSpace(mode)) }
}
func WithContext(ctx context.Context) Option { return func(c *options) { c.ctx = ctx } }
func WithDatabase(db *gorm.DB) Option        { return func(c *options) { c.db = db } }

var Exports = map[string]any{
	"SearchMemory":     SearchMemory,
	"Query":            Query,
	"AmendMemory":      AmendMemory,
	"Amend":            Amend,
	"CurrentNamespace": func() string { return "default" },
	"memoryNamespace":  WithMemoryNamespace,
	"memoryTokenLimit": WithMemoryTokenLimit,
	"memoryLimit":      WithMemoryLimit,
	"memorySearchMode": WithMemorySearchMode,
}
