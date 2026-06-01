package knowledgebench

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// BenchQuery represents a single benchmark query with expected knowledge entries.
type BenchQuery struct {
	ID       string   `json:"id"`
	Query    string   `json:"query"`
	Mode     string   `json:"mode"` // "semantic" or "keyword"
	KB       []string `json:"kb"`
	Expected []string `json:"expected"` // HiddenIndex values of expected entries
}

// LoadFixtures reads a JSONL file and returns benchmark queries.
func LoadFixtures(path string) ([]*BenchQuery, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, utils.Errorf("open fixtures file: %v", err)
	}
	defer f.Close()

	var queries []*BenchQuery
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
			continue
		}
		var q BenchQuery
		if err := json.Unmarshal([]byte(line), &q); err != nil {
			return nil, utils.Errorf("line %d: %v", lineNum, err)
		}
		queries = append(queries, &q)
	}
	if err := scanner.Err(); err != nil {
		return nil, utils.Errorf("scan: %v", err)
	}
	return queries, nil
}

// ExportQueriesFromPotentialQuestions generates benchmark queries from
// KnowledgeBaseEntry.PotentialQuestions. It picks up to `maxPerEntry`
// questions per entry, setting the entry's HiddenIndex as the expected result.
func ExportQueriesFromPotentialQuestions(db *gorm.DB, kbName string, maxPerEntry int) ([]*BenchQuery, error) {
	kb, err := yakit.GetKnowledgeBaseByName(db, kbName)
	if err != nil {
		return nil, utils.Errorf("get knowledge base %q: %v", kbName, err)
	}

	page := 1
	limit := 100
	var queries []*BenchQuery
	idx := 0

	for {
		_, entries, err := yakit.GetKnowledgeBaseEntryByFilter(db, int64(kb.ID), "", &ypb.Paging{
			Page:  int64(page),
			Limit: int64(limit),
		})
		if err != nil {
			return nil, utils.Errorf("query entries page %d: %v", page, err)
		}
		if len(entries) == 0 {
			break
		}

		for _, entry := range entries {
			if entry == nil || entry.HiddenIndex == "" {
				continue
			}
			pqs := entry.PotentialQuestions
			if len(pqs) == 0 {
				continue
			}
			count := maxPerEntry
			if count <= 0 || count > len(pqs) {
				count = len(pqs)
			}
			for i := 0; i < count; i++ {
				q := strings.TrimSpace(pqs[i])
				if q == "" {
					continue
				}
				idx++
				queries = append(queries, &BenchQuery{
					ID:       fmt.Sprintf("pq_%04d", idx),
					Query:    q,
					Mode:     "semantic",
					KB:       []string{kbName},
					Expected: []string{entry.HiddenIndex},
				})
			}
		}

		if len(entries) < limit {
			break
		}
		page++
	}

	log.Infof("exported %d benchmark queries from %q potential_questions", len(queries), kbName)
	return queries, nil
}

// SaveFixtures writes benchmark queries to a JSONL file.
func SaveFixtures(path string, queries []*BenchQuery) error {
	f, err := os.Create(path)
	if err != nil {
		return utils.Errorf("create file: %v", err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, q := range queries {
		data, err := json.Marshal(q)
		if err != nil {
			return utils.Errorf("marshal query %s: %v", q.ID, err)
		}
		w.Write(data)
		w.WriteString("\n")
	}
	return w.Flush()
}
