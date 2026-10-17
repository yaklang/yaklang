package scannode

import (
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/node"
	"strings"
	"testing"
	"time"
)

func validCompanyDispatchParams() map[string]interface{} {
	return map[string]interface{}{
		scannodeCompanyIDParamKey:         "company-1",
		scannodeTaskIDParamKey:            "job-1",
		scannodeAttemptIDParamKey:         "attempt-1",
		scannodeSSATicketRequiredParamKey: true,
		scannodeSSASkipMigrateParamKey:    true,
		scannodeSSADatabaseRawParamKey:    "postgres://scan@db.example/company_1",
		"business_input":                  "kept",
	}
}

func TestValidateCompanyDispatchParams(t *testing.T) {
	params := validCompanyDispatchParams()
	if err := validateCompanyDispatchParams("company-1", "job-1", "attempt-1", params); err != nil {
		t.Fatalf("valid company dispatch rejected: %v", err)
	}

	params[scannodeAttemptIDParamKey] = "attempt-2"
	if err := validateCompanyDispatchParams("company-1", "job-1", "attempt-1", params); err == nil || !strings.Contains(err.Error(), "attempt_id") {
		t.Fatalf("attempt mismatch error = %v", err)
	}

	params = validCompanyDispatchParams()
	params["_scannode_ssa_sts_access_key"] = "global-key"
	if err := validateCompanyDispatchParams("company-1", "job-1", "attempt-1", params); err == nil || !strings.Contains(err.Error(), "static artifact credentials") {
		t.Fatalf("static credential error = %v", err)
	}
}

func TestTakeScanNodeInternalParamsKeepsSecretsOutOfScriptInput(t *testing.T) {
	params := validCompanyDispatchParams()
	internal := takeScanNodeInternalParams(params)
	if internal[scannodeSSADatabaseRawParamKey] == nil {
		t.Fatal("SSA IR DSN was not captured for the trusted runtime")
	}
	if params["business_input"] != "kept" {
		t.Fatalf("business input changed: %#v", params)
	}
	for key := range params {
		if strings.HasPrefix(key, scannodeInternalParamPrefix) {
			t.Fatalf("internal parameter leaked to script input: %s", key)
		}
	}
}

func TestSSAArtifactTicketRequestCarriesAssignedIdentityOnly(t *testing.T) {
	raw, err := json.Marshal(ssaArtifactTicketRequest{
		NodeSessionID: "session-1",
		TaskID:        "job-1",
		AttemptID:     "attempt-1",
		ArtifactKind:  ssaArtifactTicketKindIR,
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, expected := range []string{"node_session_id", "task_id", "attempt_id", "artifact_kind"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("ticket request missing %s: %s", expected, text)
		}
	}
	if strings.Contains(text, "object_key") {
		t.Fatalf("ticket request must not contain a node-selected object key: %s", text)
	}
}

func TestValidateDispatchCommandForSessionRequiresCompanyMatch(t *testing.T) {
	command := validDispatchCommand()
	command.Metadata.CompanyId = "company-1"
	if err := validateDispatchCommandForSession("node-a", "company-1", command); err != nil {
		t.Fatalf("matching company dispatch rejected: %v", err)
	}
	command.Metadata.CompanyId = "company-2"
	if err := validateDispatchCommandForSession("node-a", "company-1", command); err == nil || !strings.Contains(err.Error(), "company_id") {
		t.Fatalf("company mismatch error = %v", err)
	}
}

func TestCompanyGenericDispatchDoesNotRequireSSAConfiguration(t *testing.T) {
	params := map[string]interface{}{scannodeCompanyIDParamKey: "company-1", scannodeTaskIDParamKey: "job-1", scannodeAttemptIDParamKey: "attempt-1"}
	if err := validateCompanyDispatchParams("company-1", "job-1", "attempt-1", params); err != nil {
		t.Fatal(err)
	}
	params["_scannode_ssa_sts_secret_key"] = "static-secret"
	if err := validateCompanyDispatchParams("company-1", "job-1", "attempt-1", params); err == nil {
		t.Fatal("accepted static secret")
	}
}

func TestCompanyTicketUsesRFC3339ExpiryAndAttemptPrefix(t *testing.T) {
	var ticket ssaArtifactTicketResponse
	if err := json.Unmarshal([]byte(`{"expires_at":"2099-01-01T00:00:00Z","object_key":"companies/company-1/jobs/job-1/attempts/attempt-1/ssa.tar.zst","endpoint":"https://store.invalid","bucket":"company-data","sts_access_key":"access","sts_secret_key":"secret","sts_session_token":"token"}`), &ticket); err != nil {
		t.Fatal(err)
	}
	session := node.SessionState{CompanyID: "company-1"}
	cfg, err := uploadConfigFromTicket(ticket, session, "job-1", "attempt-1", ssaArtifactTicketKindIR)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.STSExpiresAt != ticket.ExpiresAt.Unix() || cfg.authorizedAttemptDir != "companies/company-1/jobs/job-1/attempts/attempt-1" {
		t.Fatalf("ticket scope/expiry was lost")
	}
	ticket.ObjectKey = "companies/company-2/jobs/job-1/attempts/attempt-1/ssa.tar.zst"
	if _, err := uploadConfigFromTicket(ticket, session, "job-1", "attempt-1", ssaArtifactTicketKindIR); err == nil {
		t.Fatal("accepted cross-company ticket")
	}
	ticket.ObjectKey = "companies/company-1/jobs/job-1/attempts/attempt-2/ssa.tar.zst"
	if _, err := uploadConfigFromTicket(ticket, session, "job-1", "attempt-1", ssaArtifactTicketKindIR); err == nil {
		t.Fatal("accepted other attempt ticket")
	}
}

func TestCompanyDebugAndContinuousUploadsStayInsideAttempt(t *testing.T) {
	cfg := testSSAUploadConfig("https://objects.invalid")
	cfg.authorizedAttemptDir = "companies/company-1/jobs/job-1/attempts/attempt-1"
	cfg.ObjectKey = cfg.authorizedAttemptDir + "/ssa.tar.zst"
	cfg.STSExpiresAt = time.Now().Add(time.Hour).Unix()
	var scanNode *ScanNode
	provider := scanNode.buildDebugUploadConfigProvider(context.Background(), nil, cfg, "global/jobs/debug/profile.zip")
	debug, err := provider(false)
	if err != nil {
		t.Fatal(err)
	}
	if debug.ObjectKey != cfg.authorizedAttemptDir+"/debug/profile.zip" {
		t.Fatalf("debug key escaped attempt: %s", debug.ObjectKey)
	}
	session, err := newSSAObjectStoreUploadSession(func(bool) (*SSAArtifactUploadConfig, error) { cp := *cfg; return &cp, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, err := session.uploadBytes(context.Background(), []byte("payload"), "other-company/manifest.json"); err == nil || !strings.Contains(err.Error(), "outside the authorized attempt") {
		t.Fatalf("unscoped object was not rejected: %v", err)
	}
	cfg.authorizedAttemptDir = "companies/company-2/jobs/job-1/attempts/attempt-1"
	if err := session.refreshCredentials(true); err == nil {
		t.Fatal("refresh changed attempt boundary")
	}
}

func TestCompanyNodeRejectsGlobalCompilerEnvironment(t *testing.T) {
	t.Setenv(consts.ENV_SSA_DATABASE_RAW, "postgres://compiler:secret@db.invalid/company")
	if err := ValidateNodeDatabaseEnvironment(); err == nil {
		t.Fatal("long-lived node accepted global compiler DSN")
	}
	clean := removeEnvironmentValues([]string{"SAFE=value", consts.ENV_SSA_DATABASE_RAW + "=secret", consts.ENV_SSA_DB_SKIP_MIGRATE + "=1", consts.ENV_SSA_DATABASE_COMPANY_ID + "=company-a"}, consts.ENV_SSA_DATABASE_RAW, consts.ENV_SSA_DB_SKIP_MIGRATE, consts.ENV_SSA_DATABASE_COMPANY_ID)
	if len(clean) != 1 || clean[0] != "SAFE=value" {
		t.Fatalf("task inherited SSA credentials: %v", clean)
	}
}
