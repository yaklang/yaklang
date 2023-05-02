package cvemodels

import (
	"testing"
	"yaklang.io/yaklang/common/log"
)

func TestVersionTree(t *testing.T) {
	versions := []string{
		"1.1.1",
		"1.1.2",
		"1.1.3",
		"1.1.4",
		"1.1.5",
		"1.1.6",
		"1.1.7",
		"1.1.8",
		"1.1.9",
		"1.1.10",
		"1.1.11",
		"1.1.12",
		"1.1.13",
		"1.1.14",
		"1.1.15",
		"1.1.16",
	}

	tree := NewVersionTree("cpe:2.3:a:nginx:nginx:",
		versions...,
	)
	_ = tree
	log.Infof("strings: %v", tree.Strings())
}
