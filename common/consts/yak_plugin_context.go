package consts

import (
	"context"

	"github.com/google/uuid"
)

const (
	PLUGIN_CONTEXT_KEY_RUNTIME_ID = "PLUGIN_RUNTIME_ID"
)

func NewPluginContext() context.Context {
	return context.WithValue(context.Background(), PLUGIN_CONTEXT_KEY_RUNTIME_ID, uuid.NewString())
}
