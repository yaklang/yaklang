package syntaxflow_scan

import (
	"context"
)

func StartScan(ctx context.Context, opts ...Option) (string, error) {
	config := &Config{}
	for _, opt := range opts {
		if err := opt(config); err != nil {
			return "", err
		}
	}
	m, err := createSyntaxFlowTaskByConfig(ctx, config)
	if err != nil {
		return "", err
	}
	err = m.startScan()
	if err != nil {
		return "", err
	}
	return m.TaskId(), nil
}
