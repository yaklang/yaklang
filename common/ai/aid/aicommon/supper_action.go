package aicommon

import (
	"bytes"
	"context"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bufpipe"
	"io"
	"sync"
)

type SupperAction struct {
	ctx     context.Context
	name    string
	params  aitool.InvokeParams
	barrier *utils.CondBarrier
	mu      sync.Mutex

	wholeParamsKey string
}

func (a *SupperAction) Set(key string, value interface{}) {
	a.mu.Lock()
	a.params[key] = value
	a.mu.Unlock()
	a.barrier.CreateBarrier(key).Done()
}

func (a *SupperAction) Name() string {
	return a.name
}

func (a *SupperAction) SetName(i string) {
	a.name = i
}

func (a *SupperAction) waitKey(key ...string) {
	err := a.barrier.Wait(key...)
	if err != nil {
		log.Errorf("SupperAction waitKey %v error: %v", key, err)
	}
}

func (a *SupperAction) GetInt(key string, defaults ...int) int {
	a.waitKey(key)
	a.mu.Lock()
	defer a.mu.Unlock()
	return int(a.params.GetInt(key, lo.Map(defaults, func(item int, index int) int64 {
		return int64(item)
	})...))
}

func (a *SupperAction) GetFloat(key string, defaults ...float64) float64 {
	a.waitKey(key)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.params.GetFloat(key, defaults...)
}

func (a *SupperAction) GetString(key string, defaults ...string) string {
	a.waitKey(key)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.params.GetString(key, defaults...)
}

func (a *SupperAction) GetAnyToString(key string, defaults ...string) string {
	a.waitKey(key)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.params.GetAnyToString(key, defaults...)
}

func (a *SupperAction) GetStringSlice(key string, defaults ...[]string) []string {
	a.waitKey(key)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.params.GetStringSlice(key, defaults...)
}

func (a *SupperAction) ActionType() string {
	return a.GetString("@action")
}

func (a *SupperAction) GetBool(key string, defaults ...bool) bool {
	a.waitKey(key)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.params.GetBool(key, defaults...)
}

func (a *SupperAction) GetInvokeParams(key string) aitool.InvokeParams {
	a.waitKey(key)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.params.GetObject(key)
}

func (a *SupperAction) GetInvokeParamsArray(key string) []aitool.InvokeParams {
	a.waitKey(key)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.params.GetObjectArray(key)
}

func (a *SupperAction) GetParams() aitool.InvokeParams {
	return a.GetInvokeParams(a.wholeParamsKey)
}

type SupperActionMaker struct {
	actionName       string
	alias            []string
	jsonCallback     []jsonextractor.CallbackOption
	onReaderFinished []func()
	tagToKey         map[string]string // tag to param name mapping
	once             string

	fieldStreamHandler map[string]func(reader io.Reader)
}

type SupperActionOption func(maker *SupperActionMaker)

func WithSupperActionAlias(alias ...string) SupperActionOption {
	return func(maker *SupperActionMaker) {
		maker.alias = alias
	}
}

func WithSupperActionJSONCallback(opts ...jsonextractor.CallbackOption) SupperActionOption {
	return func(maker *SupperActionMaker) {
		maker.jsonCallback = opts
	}
}

func WithSupperActionOnReaderFinished(f ...func()) SupperActionOption {
	return func(maker *SupperActionMaker) {
		if maker.onReaderFinished == nil {
			maker.onReaderFinished = make([]func(), 0)
		}
		maker.onReaderFinished = append(maker.onReaderFinished, f...)
	}
}

func WithSupperActionTagToKey(tagName string, key string) SupperActionOption {
	return func(maker *SupperActionMaker) {
		if maker.tagToKey == nil {
			maker.tagToKey = make(map[string]string)
		}
		maker.tagToKey[tagName] = key
	}
}

func WithSupperActionOnce(once string) SupperActionOption {
	return func(maker *SupperActionMaker) {
		maker.once = once
	}
}

func (m *SupperActionMaker) ReadFromReader(ctx context.Context, reader io.Reader) *SupperAction {
	subCtx, cancel := context.WithCancel(ctx)

	onReaderFinished := m.onReaderFinished
	uuidKey := uuid.NewString()
	ac := &SupperAction{
		name:           m.actionName,
		params:         make(map[string]any),
		wholeParamsKey: uuidKey,
		ctx:            subCtx,
		barrier:        utils.NewCondBarrierContext(subCtx),
	}

	actions := []string{m.actionName}
	actions = append(actions, m.alias...)

	pipeStream := func(filedName string) *bufpipe.PipeWriter {
		handle, ok := m.fieldStreamHandler[filedName]
		if !ok {
			return nil
		}
		pr, pw := bufpipe.NewPipe()
		go func() {
			defer pr.Close()
			handle(pr)
		}()
		return pw
	}

	var tagsHandle []func(mReader io.Reader)
	for tagName, key := range m.tagToKey {
		tagsHandle = append(tagsHandle, func(mReader io.Reader) {
			err := aitag.Parse(
				utils.UTF8Reader(mReader),
				aitag.WithCallback(tagName, m.once, func(rd io.Reader) {
					var out bytes.Buffer
					writer := pipeStream(tagName)
					if writer != nil {
						defer writer.Close()
						rd = io.TeeReader(rd, writer)
					}
					_, err := io.Copy(&out, rd)
					if err != nil {
						log.Errorf("Failed to read FINAL_ANSWER for tag %s: %v", tagName, err)
						return
					}
					ac.Set(key, out.String())
				}))
			if err != nil && err != io.EOF {
				log.Warnf("Failed to read tag %s: %v", tagName, err)
			}
		})
	}

	reader = utils.CreateUTF8StreamMirror(reader, tagsHandle...)

	var err error
	var buf bytes.Buffer
	go func() {
		defer cancel()
		defer func() {
			for _, onFinished := range onReaderFinished {
				onFinished()
			}
		}()
		actionStart := utils.NewBool(false)

		opts := m.jsonCallback
		opts = append(opts, jsonextractor.WithFormatKeyValueCallback(func(key, data any) {
			if !actionStart.IsSet() {
				if utils.InterfaceToString(key) == "@action" {
					value := utils.InterfaceToString(data)
					if utils.StringArrayContains(actions, value) {
						log.Infof("matched @action: %s", value)
						ac.SetName(value)
						actionStart.Set()
					} else if mapData, ok := data.(map[string]any); ok {
						for _, v := range mapData {
							if utils.StringArrayContains(actions, utils.InterfaceToString(v)) {
								ac.SetName(utils.InterfaceToString(v))
								actionStart.Set()
								break
							}
						}
					}
				}
				return
			}
			keyString := utils.InterfaceToString(key)
			ac.Set(keyString, data)
		}))

		opts = append(opts, jsonextractor.WithObjectCallback(func(data map[string]any) { // set the whole object if @action matched
			dataParams := aitool.InvokeParams(data)
			if !dataParams.Has("@action") {
				return
			}
			targetString := dataParams.GetString("@action")
			if targetString != "" {
				if utils.StringArrayContains(actions, targetString) {
					ac.Set(uuidKey, data)
					return
				}
			} else {
				target := dataParams.GetObject("@action")
				for _, v := range target {
					if utils.StringArrayContains(actions, utils.InterfaceToString(v)) {
						ac.Set(uuidKey, data)
						return
					}
				}
			}
		}))

		err = jsonextractor.ExtractStructuredJSONFromStream(io.TeeReader(reader, &buf), opts...)
		if err != nil {
			log.Error("Failed to extract action", "action", buf.String(), "error", err)
		}
	}()

	return ac
}
