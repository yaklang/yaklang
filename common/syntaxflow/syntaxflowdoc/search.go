package syntaxflowdoc

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils/fuzzy"
)

const (
	defaultSearchLimit = 20
	maxSearchLimit     = 64
)

// SearchHit is a ranked SyntaxFlowDoc search result.
type SearchHit struct {
	Category    string
	Name        string
	Title       string
	Description string
	Example     string
	Score       float64
}

type searchIndex struct {
	keys []string
	hits map[string]*SearchHit
}

func buildSearchIndex(h *DocumentHelper) *searchIndex {
	idx := &searchIndex{
		keys: make([]string, 0),
		hits: make(map[string]*SearchHit),
	}
	if h == nil {
		return idx
	}
	for _, e := range h.AllEntries() {
		if e == nil {
			continue
		}
		parts := []string{e.Category, e.Name, e.Title, e.Description, e.Example, e.Path, e.Language}
		parts = append(parts, e.Aliases...)
		key := strings.ToLower(strings.Join(parts, " "))
		if _, exists := idx.hits[key]; exists {
			continue
		}
		idx.hits[key] = &SearchHit{
			Category:    e.Category,
			Name:        e.Name,
			Title:       e.Title,
			Description: truncateRunes(e.Description, 240),
			Example:     truncateRunes(e.Example, 160),
		}
		idx.keys = append(idx.keys, key)
	}
	return idx
}

func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}

// SearchDocument fuzzy-searches the helper. categoryFilter empty means all.
func SearchDocument(h *DocumentHelper, query string, limit int, categoryFilter string) []*SearchHit {
	query = strings.TrimSpace(query)
	if query == "" || h == nil {
		return nil
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}
	categoryFilter = strings.TrimSpace(strings.ToLower(categoryFilter))

	idx := buildSearchIndex(h)
	matches := fuzzy.RankFind(strings.ToLower(query), idx.keys)
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].Distance < matches[j].Distance
	})

	out := make([]*SearchHit, 0, limit)
	seen := make(map[string]struct{})
	for _, m := range matches {
		hit := idx.hits[m.Target]
		if hit == nil {
			continue
		}
		if categoryFilter != "" && strings.ToLower(hit.Category) != categoryFilter {
			continue
		}
		id := hit.Category + "|" + hit.Name
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		cp := *hit
		if m.Distance <= 0 {
			cp.Score = 1
		} else {
			cp.Score = 1 / (1 + float64(m.Distance))
		}
		if math.IsNaN(cp.Score) {
			cp.Score = 0
		}
		out = append(out, &cp)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// FormatSearchHits renders hits for AI / CLI consumption.
func FormatSearchHits(query string, hits []*SearchHit) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[SyntaxFlowDoc] search %q → %d hits\n\n", query, len(hits)))
	for i, h := range hits {
		if h == nil {
			continue
		}
		b.WriteString(fmt.Sprintf("%d. [%s] %s", i+1, h.Category, h.Name))
		if h.Title != "" && h.Title != h.Name {
			b.WriteString(" — " + h.Title)
		}
		b.WriteByte('\n')
		if h.Description != "" {
			b.WriteString("   " + h.Description + "\n")
		}
		if h.Example != "" {
			b.WriteString("   example: " + h.Example + "\n")
		}
	}
	return b.String()
}

var (
	defaultHelperOnce sync.Once
	defaultHelper     *DocumentHelper
)

// SetDefaultDocumentHelper overrides the process-wide helper (tests / generate inject).
func SetDefaultDocumentHelper(h *DocumentHelper) {
	defaultHelper = h
}

// GetDefaultDocumentHelper returns the embedded or last-set helper.
// Prefer doc.GetDefaultDocumentHelper in production (loads embed).
func GetDefaultDocumentHelper() *DocumentHelper {
	return defaultHelper
}
