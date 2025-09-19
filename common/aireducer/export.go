package aireducer

var Exports = map[string]any{
	"NewReducerFromReader": NewReducerFromReader,
	"NewReducerFromFile":   NewReducerFromFile,
	"NewReducerFromString": NewReducerFromString,

	//"File":                 _reducerFile,
	//"FileWithLineNumber":   _reducerFileLine,
	//"String":               _reducer,
	//"StringWithLineNumber": _reducerFileLine,
	//"Reader":               _reducer,
	//"ReaderWithLineNumber": _reducer,

	"reducerCallback":            WithReducerCallback,
	"callback":                   WithReducerCallback,
	"timeTriggerInterval":        WithTimeTriggerInterval,
	"timeTriggerIntervalSeconds": WithTimeTriggerIntervalSeconds,
	"context":                    WithContext,
	"memory":                     WithMemory,

	"separator":        WithSeparatorTrigger,
	"chunkSize":        WithChunkSize,
	"lines":            WithLines,
	"enableLineNumber": WithEnableLineNumber,
}

//func _reducer(i any, cb func(chunk chunkmaker.Chunk), options ...Option) error {
//	options = append(options, WithSimpleCallback(cb))
//
//	var r io.Reader
//	switch v := i.(type) {
//	case io.Reader:
//		r = v
//	default:
//		r = bytes.NewReader(utils.InterfaceToBytes(i))
//	}
//
//	if r == nil {
//		return utils.Errorf("input is nil")
//	}
//	reducer, err := NewReducerFromReader(r, options...)
//	if err != nil {
//		return err
//	}
//
//	if reducer.config.callback == nil {
//		return utils.Errorf("reducer callback is nil")
//	}
//	return reducer.Run()
//}
//
//func _reducerFileLine(filename string, callback func(chunk chunkmaker.Chunk), options ...Option) error {
//	reader, err := os.OpenFile(filename, os.O_RDONLY, 0644)
//	if err != nil {
//		return err
//	}
//	pr, pw, err := os.Pipe()
//	if err != nil {
//		return utils.Errorf("create pipe failed: %v", err)
//	}
//
//	go func() {
//		defer pw.Close()
//		defer reader.Close()
//
//		lineReader := utils.PrefixLinesWithLineNumbersReader(reader)
//		io.Copy(pw, lineReader)
//	}()
//
//	return _reducer(pr, callback, options...)
//}
//
//func _reducerFile(filename string, i func(chunk chunkmaker.Chunk), options ...Option) error {
//	options = append(options, WithSimpleCallback(i))
//	r, err := NewReducerFromFile(filename, options...)
//	if err != nil {
//		return err
//	}
//	if r.config.callback == nil {
//		return nil
//	}
//	return r.Run()
//}
