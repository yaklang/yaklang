package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/license"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
)

// TestProductsFlagEmbedsEntitlements verifies that signing with --products
// embeds the entitlements JSON in license.Params, decodable by the license
// Machine.
func TestProductsFlagEmbedsEntitlements(t *testing.T) {
	pri, pub, err := tlsutils.GeneratePrivateAndPublicKeyPEM()
	if err != nil {
		t.Fatalf("gen keypair: %v", err)
	}
	tmpDir := t.TempDir()
	priPath := filepath.Join(tmpDir, "pri.pem")
	pubPath := filepath.Join(tmpDir, "pub.pem")
	if err := os.WriteFile(priPath, pri, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pubPath, pub, 0o600); err != nil {
		t.Fatal(err)
	}

	machine, err := license.NewMachineFromFile(pubPath, priPath)
	if err != nil {
		t.Fatalf("new machine: %v", err)
	}
	req, err := machine.GenerateRequest()
	if err != nil {
		t.Fatalf("gen request: %v", err)
	}
	reqPath := filepath.Join(tmpDir, "req.txt")
	if err := os.WriteFile(reqPath, []byte(req), 0o600); err != nil {
		t.Fatal(err)
	}

	binPath := filepath.Join(tmpDir, "xlic")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build xlic: %v\n%s", err, out)
	}

	signCmd := exec.Command(binPath,
		"--enc", pubPath, "--dec", priPath,
		"--req", reqPath, "--org", "Acme", "-d", "30",
		"--products", "hids,ssa")
	// CLI writes via Go's println → stderr. CombinedOutput captures it.
	out, err := signCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sign: %v\n%s", err, out)
	}
	// The signed license is the longest hex-looking line between dashed lines.
	signed := extractSignedLicense(string(out))
	if signed == "" {
		t.Fatalf("could not extract signed license from output:\n%s", out)
	}

	resp, err := machine.VerifyLicense(signed)
	if err != nil {
		t.Fatalf("verify signed license: %v", err)
	}
	if resp.Org != "Acme" {
		t.Errorf("org = %q, want Acme", resp.Org)
	}
	entRaw, ok := resp.Params["entitlements"]
	if !ok {
		t.Fatal("entitlements key missing from license params")
	}
	var ent struct {
		Products []string `json:"products"`
		Version  int      `json:"version"`
	}
	if err := json.Unmarshal([]byte(entRaw), &ent); err != nil {
		t.Fatalf("decode entitlements: %v", err)
	}
	if len(ent.Products) != 2 || ent.Products[0] != "hids" || ent.Products[1] != "ssa" {
		t.Errorf("products = %v, want [hids ssa]", ent.Products)
	}
	if ent.Version != 1 {
		t.Errorf("version = %d, want 1", ent.Version)
	}
}

// TestProductsFlagRejectsUnknownKey verifies a typo is reported in the output.
// (The CLI prints the error but does not exit non-zero — a pre-existing
// behavior of this CLI — so we assert on the message, not the exit code.)
func TestProductsFlagRejectsUnknownKey(t *testing.T) {
	pri, pub, err := tlsutils.GeneratePrivateAndPublicKeyPEM()
	if err != nil {
		t.Fatalf("gen keypair: %v", err)
	}
	tmpDir := t.TempDir()
	priPath := filepath.Join(tmpDir, "pri.pem")
	pubPath := filepath.Join(tmpDir, "pub.pem")
	_ = os.WriteFile(priPath, pri, 0o600)
	_ = os.WriteFile(pubPath, pub, 0o600)
	machine, err := license.NewMachineFromFile(pubPath, priPath)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := machine.GenerateRequest()
	reqPath := filepath.Join(tmpDir, "req.txt")
	_ = os.WriteFile(reqPath, []byte(req), 0o600)

	binPath := filepath.Join(tmpDir, "xlic")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	signCmd := exec.Command(binPath,
		"--enc", pubPath, "--dec", priPath,
		"--req", reqPath, "--org", "Acme", "-d", "30",
		"--products", "hids,bogus")
	out, _ := signCmd.CombinedOutput()
	if !strings.Contains(string(out), "unknown product key") {
		t.Errorf("expected 'unknown product key' in output, got: %s", out)
	}
}

// extractSignedLicense pulls the hex license string out of the CLI's
// dashed-line-delimited output. The license is the multi-line hex blob
// (lines joined by newlines in the yaklang format) between the two dashed
// separator lines.
func extractSignedLicense(output string) string {
	lines := strings.Split(output, "\n")
	start := -1
	for i, l := range lines {
		if strings.Contains(l, "----") {
			start = i + 1
			break
		}
	}
	if start < 0 || start >= len(lines) {
		return ""
	}
	var collected []string
	for i := start; i < len(lines); i++ {
		if strings.Contains(lines[i], "----") {
			break
		}
		l := strings.TrimSpace(lines[i])
		if l != "" {
			collected = append(collected, l)
		}
	}
	return strings.Join(collected, "\n")
}
