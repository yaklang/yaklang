package cve

import (
	"testing"
	"github.com/yaklang/yaklang/common/cve/cvequeryops"
)

func TestUpdate(t *testing.T) {
	cvequeryops.DownLoad("/tmp/cve")
	cvequeryops.LoadCVE("/tmp/cve", "/tmp/default-cve.db")
}
