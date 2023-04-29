package cve

import (
	"yaklang/common/cve/cvequeryops"
	"testing"
)

func TestUpdate(t *testing.T) {
	cvequeryops.DownLoad("/tmp/cve")
	cvequeryops.LoadCVE("/tmp/cve", "/tmp/default-cve.db")
}
