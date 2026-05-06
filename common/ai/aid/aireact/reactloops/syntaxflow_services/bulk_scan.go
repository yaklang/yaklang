package syntaxflow_services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// BulkProjectRun is one project path started under a campaign.
type BulkProjectRun struct {
	Path       string `json:"path"`
	TaskID     string `json:"task_id,omitempty"`
	ErrMsg     string `json:"err,omitempty"`
	LastStatus string `json:"last_status,omitempty"`
	LastRisk   int64 `json:"risk_count,omitempty"`
}

// BulkCampaign describes a multi-project SyntaxFlow rule rollout backed by real background scan tasks.
type BulkCampaign struct {
	ID           string           `json:"id"`
	RulePath     string           `json:"rule_path"`
	SelectorJSON string           `json:"selector_json"`
	CreatedAt    time.Time        `json:"created_at"`
	Status       string           `json:"status"`
	Errors       []string         `json:"errors,omitempty"`
	Runs         []BulkProjectRun `json:"runs,omitempty"`
}

var bulkMu sync.Mutex
var bulkStore = make(map[string]*BulkCampaign)

// CreateCampaign registers a campaign id.
func CreateCampaign(c BulkCampaign) string {
	bulkMu.Lock()
	defer bulkMu.Unlock()
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	if c.Status == "" {
		c.Status = "pending"
	}
	cp := c
	bulkStore[c.ID] = &cp
	return c.ID
}

func updateCampaign(id string, fn func(*BulkCampaign)) {
	bulkMu.Lock()
	defer bulkMu.Unlock()
	c, ok := bulkStore[id]
	if !ok || c == nil {
		return
	}
	fn(c)
	cp := *c
	bulkStore[id] = &cp
}

// GetCampaign returns a registered campaign (copy of stored pointer fields may be shared; treat as read-mostly).
func GetCampaign(id string) (*BulkCampaign, error) {
	bulkMu.Lock()
	defer bulkMu.Unlock()
	c, ok := bulkStore[id]
	if !ok || c == nil {
		return nil, fmt.Errorf("campaign not found: %s", id)
	}
	cp := *c
	return &cp, nil
}

// BulkScanService wraps bulk registry + kickoff helpers.
type BulkScanService struct{}

// RunRuleAcrossProjects compiles each local project path, starts one SyntaxFlow scan per path with the given rule file inlined.
// selectorJSON format: {"project_paths":["/abs/a","/abs/b"]}.
func (BulkScanService) RunRuleAcrossProjects(ctx context.Context, rulePath, selectorJSON string) (campaignID string, err error) {
	return RunRuleAcrossProjects(ctx, rulePath, selectorJSON)
}

// PollCampaign refreshes per-project task status from the profile DB and aggregates campaign status.
func (BulkScanService) PollCampaign(db *gorm.DB, id string) *BulkCampaign {
	return PollCampaign(db, id)
}

// RunRuleAcrossProjects is the package-level entry used by orchestrators.
func RunRuleAcrossProjects(ctx context.Context, rulePath, selectorJSON string) (campaignID string, err error) {
	rulePath = strings.TrimSpace(rulePath)
	if rulePath == "" {
		return "", utils.Error("empty rule path")
	}
	if _, err := os.Stat(rulePath); err != nil {
		return "", err
	}
	paths, err := parseProjectPathsSelector(selectorJSON)
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", utils.Error("selector has no project_paths")
	}
	cid := CreateCampaign(BulkCampaign{
		RulePath:     rulePath,
		SelectorJSON: selectorJSON,
		Status:       "running",
	})
	var runs []BulkProjectRun
	var runMu sync.Mutex
	var aggErrs []string
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	for _, p := range paths {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				runMu.Lock()
				runs = append(runs, BulkProjectRun{Path: p, ErrMsg: ctx.Err().Error()})
				runMu.Unlock()
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			j, err := BuildCodeScanJSONForLocalPath(p)
			if err != nil {
				runMu.Lock()
				runs = append(runs, BulkProjectRun{Path: p, ErrMsg: err.Error()})
				aggErrs = append(aggErrs, fmt.Sprintf("%s: build json: %v", p, err))
				runMu.Unlock()
				return
			}
			cfg, progs, err := LoadProgramsFromCodeScanJSON(ctx, []byte(j))
			if err != nil {
				runMu.Lock()
				runs = append(runs, BulkProjectRun{Path: p, ErrMsg: err.Error()})
				aggErrs = append(aggErrs, fmt.Sprintf("%s: load programs: %v", p, err))
				runMu.Unlock()
				return
			}
			tid, err := StartSyntaxFlowScanBackgroundWithRuleFile(ctx, cfg, progs, rulePath)
			if err != nil {
				runMu.Lock()
				runs = append(runs, BulkProjectRun{Path: p, ErrMsg: err.Error()})
				aggErrs = append(aggErrs, fmt.Sprintf("%s: start scan: %v", p, err))
				runMu.Unlock()
				return
			}
			runMu.Lock()
			runs = append(runs, BulkProjectRun{Path: p, TaskID: tid, LastStatus: schema.SYNTAXFLOWSCAN_EXECUTING})
			runMu.Unlock()
		}()
	}
	wg.Wait()
	finalStatus := "done"
	if len(aggErrs) > 0 {
		finalStatus = "partial"
	}
	updateCampaign(cid, func(c *BulkCampaign) {
		c.Runs = runs
		c.Errors = aggErrs
		c.Status = finalStatus
	})
	return cid, nil
}

type pathsSelector struct {
	ProjectPaths []string `json:"project_paths"`
}

func parseProjectPathsSelector(selectorJSON string) ([]string, error) {
	sel := strings.TrimSpace(selectorJSON)
	if sel == "" {
		return nil, utils.Error("empty selector json")
	}
	var ps pathsSelector
	if err := json.Unmarshal([]byte(sel), &ps); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ps.ProjectPaths))
	for _, p := range ps.ProjectPaths {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out, nil
}

// PollCampaign loads current campaign and refreshes each task row from DB when possible.
func PollCampaign(db *gorm.DB, id string) *BulkCampaign {
	c, err := GetCampaign(id)
	if err != nil || c == nil {
		return nil
	}
	if db == nil {
		return c
	}
	for i := range c.Runs {
		tid := strings.TrimSpace(c.Runs[i].TaskID)
		if tid == "" {
			continue
		}
		st, err := schema.GetSyntaxFlowScanTaskById(db, tid)
		if err != nil || st == nil {
			continue
		}
		c.Runs[i].LastStatus = st.Status
		c.Runs[i].LastRisk = st.RiskCount
	}
	anyExec := false
	anyErr := false
	for _, r := range c.Runs {
		if r.ErrMsg != "" {
			anyErr = true
		}
		if r.TaskID != "" && r.LastStatus == schema.SYNTAXFLOWSCAN_EXECUTING {
			anyExec = true
		}
	}
	if anyExec {
		c.Status = "running"
	} else if anyErr {
		c.Status = "partial"
	} else {
		c.Status = "done"
	}
	updateCampaign(id, func(stored *BulkCampaign) {
		stored.Runs = c.Runs
		stored.Status = c.Status
	})
	cp, _ := GetCampaign(id)
	return cp
}
