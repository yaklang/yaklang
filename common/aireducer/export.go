package aireducer

var Exports = map[string]any{
	"NewReducerFromReader": NewReducerFromReader,
	"NewReducerFromFile":   NewReducerFromFile,
	"NewReducerFromString": NewReducerFromString,

	"reducerCallback":            WithReducerCallback,
	"callback":                   WithReducerCallback,
	"timeTriggerInterval":        WithTimeTriggerInterval,
	"timeTriggerIntervalSeconds": WithTimeTriggerIntervalSeconds,
	"chunkSize":                  WithChunkSize,
	"context":                    WithContext,
	"memory":                     WithMemory,
	"separator":                  WithSeparatorTrigger,
}
