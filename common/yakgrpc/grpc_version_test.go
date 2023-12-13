package yakgrpc

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func yakVersionAtLeast(t *testing.T, local ypb.YakClient, version1, version2 string) bool {
	t.Helper()
	rsp, err := local.YakVersionAtLeast(context.Background(), &ypb.YakVersionAtLeastRequest{
		YakVersion:     version1,
		AtLeastVersion: version2,
	})
	if err != nil {
		t.Fatal(err)
	}
	return rsp.Ok
}

func TestYakVersionAtLeast(t *testing.T) {
	local, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	if !yakVersionAtLeast(t, local, "dev", "0.01") {
		t.Fatal("dev should pass")
	}
	if !yakVersionAtLeast(t, local, "v1.2.9-sp3", "v1.2.9-sp3") {
		t.Fatal("v1.2.9-sp3 should at least v1.2.9-sp3")
	}

	if !yakVersionAtLeast(t, local, "v1.2.10", "v1.2.9-sp3") {
		t.Fatal("v1.2.10 should at least v1.2.9-sp3")
	}
	if yakVersionAtLeast(t, local, "v1.2.9-sp2", "v1.2.9-sp3") {
		t.Fatal("v1.2.9-sp2 should not at least v1.2.9-sp3")
	}
	if yakVersionAtLeast(t, local, "v1.2.0", "v1.2.9-sp3") {
		t.Fatal("v1.2.0 should not at least v1.2.9-sp3")
	}
}
