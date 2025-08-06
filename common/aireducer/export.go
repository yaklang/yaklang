package aireducer

var Exports = map[string]any{
	"NewReducerFromReader": NewReducerFromReader,
	"NewReducerFromFile":   NewReducerFromFile,
	"NewReducerFromString": NewReducerFromString,

	"reducerCallback":            WithReducerCallback,
	"callback":                   WithReducerCallback,
	"timeTriggerInterval":        WithTimeTriggerInterval,
	"timeTriggerIntervalSeconds": WithTimeTriggerIntervalSeconds,
	"context":                    WithContext,
	"memory":                     WithMemory,

	"separator": WithSeparatorTrigger,
	"chunkSize": WithChunkSize,
}
