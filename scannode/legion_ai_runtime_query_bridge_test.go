package scannode

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

func TestValidateAIHTTPFlowsQueryCommand(t *testing.T) {
	valid := func() *aiv1.QueryAIHTTPFlowsCommand {
		return &aiv1.QueryAIHTTPFlowsCommand{
			Metadata:     &nodev1.CommandMetadata{CommandId: "cmd-1"},
			TargetNodeId: "node-a",
			OwnerUserId:  "user-a",
			RuntimeId:    "runtime-a",
			HiddenIndex:  "hidden-a",
		}
	}

	tests := []struct {
		name    string
		mutate  func(*aiv1.QueryAIHTTPFlowsCommand)
		wantErr string
	}{
		{name: "accepts valid command"},
		{
			name: "requires metadata",
			mutate: func(command *aiv1.QueryAIHTTPFlowsCommand) {
				command.Metadata = nil
			},
			wantErr: "metadata is required",
		},
		{
			name: "requires target node match",
			mutate: func(command *aiv1.QueryAIHTTPFlowsCommand) {
				command.TargetNodeId = "node-b"
			},
			wantErr: "target_node_id mismatch",
		},
		{
			name: "requires owner user id",
			mutate: func(command *aiv1.QueryAIHTTPFlowsCommand) {
				command.OwnerUserId = " "
			},
			wantErr: "owner_user_id is required",
		},
		{
			name: "requires runtime or hidden index",
			mutate: func(command *aiv1.QueryAIHTTPFlowsCommand) {
				command.RuntimeId = " "
				command.HiddenIndex = " "
			},
			wantErr: "runtime_id or hidden_index is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := valid()
			if tt.mutate != nil {
				tt.mutate(command)
			}

			err := validateAIHTTPFlowsQueryCommand("node-a", command)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateAIRisksQueryCommand(t *testing.T) {
	valid := func() *aiv1.QueryAIRisksCommand {
		return &aiv1.QueryAIRisksCommand{
			Metadata:     &nodev1.CommandMetadata{CommandId: "cmd-1"},
			TargetNodeId: "node-a",
			OwnerUserId:  "user-a",
			RuntimeId:    "runtime-a",
			RiskId:       7,
		}
	}

	tests := []struct {
		name    string
		mutate  func(*aiv1.QueryAIRisksCommand)
		wantErr string
	}{
		{name: "accepts valid command"},
		{
			name: "requires metadata",
			mutate: func(command *aiv1.QueryAIRisksCommand) {
				command.Metadata = nil
			},
			wantErr: "metadata is required",
		},
		{
			name: "requires target node match",
			mutate: func(command *aiv1.QueryAIRisksCommand) {
				command.TargetNodeId = "node-b"
			},
			wantErr: "target_node_id mismatch",
		},
		{
			name: "requires runtime or risk id",
			mutate: func(command *aiv1.QueryAIRisksCommand) {
				command.RuntimeId = " "
				command.RiskId = 0
			},
			wantErr: "runtime_id or risk_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := valid()
			if tt.mutate != nil {
				tt.mutate(command)
			}

			err := validateAIRisksQueryCommand("node-a", command)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestAIRuntimeQueryRefsTrimCommandMetadata(t *testing.T) {
	httpRef := aiRuntimeQueryRefFromHTTPFlowsCommand(&aiv1.QueryAIHTTPFlowsCommand{
		Metadata:    &nodev1.CommandMetadata{CommandId: " cmd-1 "},
		OwnerUserId: " user-a ",
	})
	if httpRef.CommandID != "cmd-1" || httpRef.OwnerUserID != "user-a" {
		t.Fatalf("unexpected http ref: %#v", httpRef)
	}

	riskRef := aiRuntimeQueryRefFromRisksCommand(&aiv1.QueryAIRisksCommand{
		Metadata:    &nodev1.CommandMetadata{CommandId: " cmd-2 "},
		OwnerUserId: " user-b ",
	})
	if riskRef.CommandID != "cmd-2" || riskRef.OwnerUserID != "user-b" {
		t.Fatalf("unexpected risk ref: %#v", riskRef)
	}
}

func TestNormalizeAIRuntimeQueryLimit(t *testing.T) {
	if got := normalizeAIRuntimeQueryLimit(nil); got != defaultAIRuntimeQueryLimit {
		t.Fatalf("unexpected nil limit: %d", got)
	}
	if got := normalizeAIRuntimeQueryLimit(&aiv1.AIRuntimePagination{Limit: -1}); got != defaultAIRuntimeQueryLimit {
		t.Fatalf("unexpected negative limit: %d", got)
	}
	if got := normalizeAIRuntimeQueryLimit(&aiv1.AIRuntimePagination{Limit: 500}); got != maxAIRuntimeQueryLimit {
		t.Fatalf("unexpected capped limit: %d", got)
	}
	if got := normalizeAIRuntimeQueryLimit(&aiv1.AIRuntimePagination{Limit: 12}); got != 12 {
		t.Fatalf("unexpected explicit limit: %d", got)
	}
}

func TestNormalizeAIRuntimeQueryPagination(t *testing.T) {
	page, limit, offset := normalizeAIRuntimeQueryPagination(&aiv1.AIRuntimePagination{Page: 3, Limit: 25})
	if page != 3 || limit != 25 || offset != 50 {
		t.Fatalf("unexpected pagination: page=%d limit=%d offset=%d", page, limit, offset)
	}

	page, limit, offset = normalizeAIRuntimeQueryPagination(&aiv1.AIRuntimePagination{Page: -1, Limit: -1})
	if page != 1 || limit != defaultAIRuntimeQueryLimit || offset != 0 {
		t.Fatalf("unexpected default pagination: page=%d limit=%d offset=%d", page, limit, offset)
	}
}

func TestBuildAIHTTPFlowQueryAppliesYakitTableFilters(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	query := buildAIHTTPFlowQuery(db, &aiv1.QueryAIHTTPFlowsCommand{
		RuntimeId:   "runtime-1",
		HiddenIndex: "hidden-1",
		Method:      "POST",
		StatusCode:  201,
		ContentType: "application/json",
		Keyword:     "login",
	})

	sql := strings.ToLower(fmt.Sprint(query.QueryExpr()))
	for _, want := range []string{
		"runtime_id",
		"hidden_index",
		"method",
		"status_code",
		"content_type",
		"like",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected sql to contain %q, got %s", want, sql)
		}
	}
}

func TestBuildAIRiskQueryAppliesYakitTableFilters(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	query := buildAIRiskQuery(db, &aiv1.QueryAIRisksCommand{
		RuntimeId: "runtime-1",
		RiskId:    9,
		Severity:  "high",
		RiskType:  "sqli",
		Network:   "10.0.0.1",
		Title:     "SQL",
		Keyword:   "payload",
	})

	sql := strings.ToLower(fmt.Sprint(query.QueryExpr()))
	for _, want := range []string{
		"runtime_id",
		"id",
		"severity",
		"risk_type",
		"ip",
		"title",
		"like",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected sql to contain %q, got %s", want, sql)
		}
	}
}
