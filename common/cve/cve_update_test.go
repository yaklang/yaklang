package cve

import (
	"testing"
	"yaklang/common/cve/cvequeryops"
)

func TestUpdate(t *testing.T) {
	cvequeryops.DownLoad("/tmp/cve")
	cvequeryops.LoadCVE("/tmp/cve", "/tmp/default-cve.db")
}
