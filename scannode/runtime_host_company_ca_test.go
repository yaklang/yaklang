package scannode

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/dockerhttp"
	"github.com/yaklang/yaklang/common/node"
)

func runtimeCompanyCA(t *testing.T) string {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, public, private)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err = os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestCompanyRuntimeDockerMountsOnlyConfiguredCA(t *testing.T) {
	path := runtimeCompanyCA(t)
	var request dockerhttp.ContainerCreateRequest
	d := testRuntimeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/create"):
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			w.Write([]byte(`{"Id":"runtime"}`))
		case strings.HasSuffix(r.URL.Path, "/start"):
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/json"):
			w.Write([]byte(`{"Id":"runtime","State":{"Running":true}}`))
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	})
	_, err := d.CreateAndStart(context.Background(), runtimeHostContainerInput{Name: "runtime", Image: "image", NATSCAFile: path})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(request.HostConfig.Binds, []string{path + ":/etc/legion/nats-ca.pem:ro"}) {
		t.Fatalf("unexpected trust mounts: %v", request.HostConfig.Binds)
	}
	if err = os.WriteFile(path, []byte("-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = d.CreateAndStart(context.Background(), runtimeHostContainerInput{NATSCAFile: path}); err == nil {
		t.Fatal("private material was mounted as public CA")
	}
}
func TestCompanyRuntimeHostRejectsCommandSelectedCAAndForeignCompany(t *testing.T) {
	executor := newRuntimeHostTestExecutor(t, &runtimeHostDockerStub{}, t.TempDir())
	executor.natsCAFile = runtimeCompanyCA(t)
	executor.sessionProvider = func() (node.SessionState, bool) { return node.SessionState{CompanyID: "a"}, true }
	command := runtimeHostTestCommand(t)
	spec := command.Container
	spec.Environment["LEGION_COMPANY_ID"] = "a"
	spec.Environment["LEGION_NATS_CA"] = runtimeHostContainerCAPath
	spec.Environment["SSL_CERT_FILE"] = runtimeHostContainerCAPath
	spec.Environment["LEGION_NATS_URL"] = "tls://broker.example.test:4222"
	if err := executor.validateContainerSpec(spec, "session-1"); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct{ key, value string }{{"LEGION_NATS_CA", "/operator/control-plane.creds"}, {"SSL_CERT_FILE", "/operator/control-plane.creds"}, {"LEGION_COMPANY_ID", "b"}, {"LEGION_NATS_URL", "nats://broker.example.test:4222"}, {"LEGION_NATS_CREDS", "/operator/control-plane.creds"}} {
		original, exists := spec.Environment[change.key]
		spec.Environment[change.key] = change.value
		if err := executor.validateContainerSpec(spec, "session-1"); err == nil {
			t.Errorf("accepted %s", change.key)
		}
		if exists {
			spec.Environment[change.key] = original
		} else {
			delete(spec.Environment, change.key)
		}
	}
}
