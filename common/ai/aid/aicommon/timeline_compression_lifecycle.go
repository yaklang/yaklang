package aicommon

import (
	"context"
)

func (m *Timeline) prepareCompressionOptions(options TimelineCompressionOptions) TimelineCompressionOptions {
	if options.Context == nil {
		if config := m.compressionCallerConfig(); config != nil {
			options.Context = config.GetContext()
		}
		if options.Context == nil {
			options.Context = context.Background()
		}
	}
	notify := options.OnProgress
	var lastStage TimelineCompressionStage
	options.OnProgress = func(stage TimelineCompressionStage) {
		if notify != nil && stage != lastStage {
			lastStage = stage
			notify(stage)
		}
	}
	return options
}
