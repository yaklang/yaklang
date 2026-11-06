package pcapx

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

type fakeCaptureOptionProvider struct {
	option pcaputil.CaptureOption
}

func (f fakeCaptureOptionProvider) CaptureOption() pcaputil.CaptureOption {
	return f.option
}

func TestWithSuricataEngineAppliesProviderOption(t *testing.T) {
	applied := false
	provider := fakeCaptureOptionProvider{option: func(*pcaputil.CaptureConfig) error {
		applied = true
		return nil
	}}
	require.NoError(t, WithSuricataEngine(provider)(pcaputil.NewDefaultConfig()))
	require.True(t, applied)
}

func TestWithSuricataEngineRejectsNil(t *testing.T) {
	require.Error(t, WithSuricataEngine(nil)(pcaputil.NewDefaultConfig()))
}
