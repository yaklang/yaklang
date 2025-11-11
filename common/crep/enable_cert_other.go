//go:build !darwin && !linux && !windows

package crep

import (
	"github.com/yaklang/yaklang/common/utils"
)

// AddMITMRootCertIntoSystem 不支持的平台
func AddMITMRootCertIntoSystem() error {
	return utils.Errorf("adding MITM root certificate to system trust store is not supported on this platform")
}

// WithdrawMITMRootCertFromSystem 不支持的平台
func WithdrawMITMRootCertFromSystem() error {
	return utils.Errorf("removing MITM root certificate from system trust store is not supported on this platform")
}
