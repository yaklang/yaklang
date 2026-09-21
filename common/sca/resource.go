package sca

import (
	"github.com/yaklang/yaklang/common/sca/core/budget"
)

type ResourceLimits = budget.Limits

func WithResourceLimits(l ResourceLimits) ScanOption { return func(c *ScanConfig) { c.limits = l } }
func WithSnapshotID(id string) ScanOption            { return func(c *ScanConfig) { c.snapshot = id } }
