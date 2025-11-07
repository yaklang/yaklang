package aicommon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bufpipe"
)

var ActionMagicKey = "@action"

type Action struct {
	// meta data
	name            string
	mu              sync.Mutex
	params          aitool.InvokeParams
	generalParamKey string

	// status
	streamFinish context.Context
	parseFinish  context.Context // params parsed finish condition is reader close and ai tag parse close
	barrier      *utils.CondBarrier
}

// ValidCheck 检查 检查当前 Action是否有效，主要检查是否有合法的 @action 字段 即 action type
func (a *Action) ValidCheck(expectName ...string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.params == nil {
		return false
	}
	if expectName != nil && len(expectName) > 0 {
		return utils.StringArrayContains(expectName, a.params.GetString(ActionMagicKey))
	} else {
		return a.params.GetString(ActionMagicKey) != ""
	}
}

func (a *Action) WaitParse(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-a.parseFinish.Done():
	}
	return
}

func (a *Action) WaitStream(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-a.streamFinish.Done():

	}
	return
}

func (a *Action) Set(key string, value interface{}) {
	a.mu.Lock()
	if _, ok := a.params[key]; !ok {
		a.params[key] = value
	}
	a.mu.Unlock()
	a.barrier.CreateBarrier(key).Done()
}

func (a *Action) ForceSet(key string, value interface{}) {
	a.mu.Lock()
	a.params[key] = value
	a.mu.Unlock()
	a.barrier.CreateBarrier(key).Done()
}

func (a *Action) Name() string {
	return a.name
}

func (a *Action) SetName(i string) {
	a.name = i
}

func (a *Action) waitKey(key ...string) {
	err := a.barrier.Wait(key...)
	if err != nil {
		log.Debugf("action wait key %v error: %v", key, err)
	}
}

func (a *Action) GetInt(key string, defaults ...int) int {
	a.waitKey(key)
	a.mu.Lock()
	defer a.mu.Unlock()
	return int(a.params.GetInt(key, lo.Map(defaults, func(item int, index int) int64 {
		return int64(item)
	})...))
}

func (a *Action) GetFloat(key string, defaults ...float64) float64 {
	a.waitKey(key)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.params.GetFloat(key, defaults...)
}

func (a *Action) GetString(key string, defaults ...string) string {
	if a == nil {
		return ""
	}
	a.waitKey(key)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.params.GetString(key, defaults...)
}

func (a *Action) GetAnyToString(key string, defaults ...string) string {
	a.waitKey(key)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.params.GetAnyToString(key, defaults...)
}

func (a *Action) GetStringSlice(key string, defaults ...[]string) []string {
	a.waitKey(key)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.params.GetStringSlice(key, defaults...)
}

func (a *Action) ActionType() string {
	return a.GetString("@action")
}

func (a *Action) GetBool(key string, defaults ...bool) bool {
	a.waitKey(key)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.params.GetBool(key, defaults...)
}

func (a *Action) GetInvokeParams(key string) aitool.InvokeParams {
	a.waitKey(key)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.params.GetObject(key)
}

func (a *Action) GetInvokeParamsArray(key string) []aitool.InvokeParams {
	a.waitKey(key)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.params.GetObjectArray(key)
}

func (a *Action) GetParams() aitool.InvokeParams {
	return a.GetInvokeParams(a.generalParamKey)
}

type ActionMaker struct {
	actionName       string
	alias            []string
	jsonCallback     []jsonextractor.CallbackOption
	onReaderFinished []func()
	tagToKey         map[string]string // tag to param name mapping
	nonce            string

	fieldStreamHandler []*FieldStreamItem
}
type FieldStreamItem struct {
	FieldName []string
	Handler   func(key string, reader io.Reader)
}

type ActionMakerOption func(maker *ActionMaker)

func WithActionAlias(alias ...string) ActionMakerOption {
	return func(maker *ActionMaker) {
		maker.alias = alias
	}
}

func WithActionJSONCallback(opts ...jsonextractor.CallbackOption) ActionMakerOption {
	return func(maker *ActionMaker) {
		maker.jsonCallback = opts
	}
}

func WithActionOnReaderFinished(f ...func()) ActionMakerOption {
	return func(maker *ActionMaker) {
		if maker.onReaderFinished == nil {
			maker.onReaderFinished = make([]func(), 0)
		}
		maker.onReaderFinished = append(maker.onReaderFinished, f...)
	}
}

func WithActionTagToKey(tagName string, key string) ActionMakerOption {
	return func(maker *ActionMaker) {
		if maker.tagToKey == nil {
			maker.tagToKey = make(map[string]string)
		}
		maker.tagToKey[tagName] = key
	}
}

func WithActionNonce(nonce string) ActionMakerOption {
	return func(maker *ActionMaker) {
		maker.nonce = nonce
	}
}

func WithActionFieldStreamHandler(fieldNames []string, handler func(key string, r io.Reader)) ActionMakerOption {
	return func(maker *ActionMaker) {
		maker.fieldStreamHandler = append(maker.fieldStreamHandler, &FieldStreamItem{
			FieldName: fieldNames,
			Handler:   handler,
		})
	}
}

func (m *ActionMaker) ReadFromReader(ctx context.Context, reader io.Reader) *Action {
	streamCtx, streamFinish := context.WithCancel(ctx)
	parseCtx, parseFinish := context.WithCancel(ctx) //  barrier use this ctx ,because parse finish means all params are ready

	generalParamsKey := uuid.NewString()
	action := &Action{
		name:            m.actionName,
		params:          make(map[string]any),
		generalParamKey: generalParamsKey,
		barrier:         utils.NewCondBarrierContext(parseCtx),
		parseFinish:     parseCtx,
		streamFinish:    streamCtx,
	}

	actionNames := []string{m.actionName}
	actionNames = append(actionNames, m.alias...)

	fieldHandlerMap := make(map[string][]func(key string, r io.Reader))
	for _, streamItem := range m.fieldStreamHandler {
		for _, fieldName := range streamItem.FieldName {
			if _, ok := fieldHandlerMap[fieldName]; !ok {
				fieldHandlerMap[fieldName] = make([]func(key string, r io.Reader), 0)
			}
			fieldHandlerMap[fieldName] = append(fieldHandlerMap[fieldName], streamItem.Handler)
		}
	}

	streamWg := sync.WaitGroup{}

	mirrorPipe := func(filedName string) *bufpipe.PipeWriter {
		handle, ok := fieldHandlerMap[filedName]
		if !ok {
			return nil
		}
		prs, pw := utils.NewMirrorPipe(len(handle))
		if len(prs) < len(handle) {
			log.Errorf("Field stream handler count mismatch for field %s", filedName)
			return nil
		}
		for i, h := range handle {
			if i >= len(prs) {
				log.Errorf("Field stream handler index out of range for field %s", filedName)
				return nil
			}
			streamWg.Add(1)
			go func(reader io.Reader, callback func(key string, r io.Reader)) {
				defer func() {
					streamWg.Done()
				}()
				callback(filedName, reader)
			}(prs[i], h)
		}
		return pw
	}

	parserWG := sync.WaitGroup{}

	// make tag parsers
	var tagsParseHandles []func(mReader io.Reader)
	for tagName, fieldName := range m.tagToKey {
		tagName := tagName
		fieldName := fieldName
		parserWG.Add(1)
		handle := func(mReader io.Reader) {
			mirrorStart := time.Now()
			log.Debugf("[ai-tag] mirror[%s] started, time since mirror start: %v", tagName, time.Since(mirrorStart))
			pReader := utils.NewPeekableReader(mReader)
			parseStart := time.Now()
			pReader.Peek(1)
			log.Debugf("[ai-tag] mirror peeked first byte for tag[%s] cost: %v", tagName, time.Since(parseStart))
			log.Debugf("starting aitag.Parse for tag[%s]", tagName)
			defer func() {
				cost := time.Since(parseStart)
				log.Debugf("finished aitag.Parse for tag[%s], total cost: %v", tagName, cost)
				if cost.Milliseconds() <= 300 {
					log.Debugf("AI Response Mirror[%s] stream too fast, cost %v, stream maybe not valid", tagName, cost)
				} else {
					log.Debugf("AI Response Mirror[%s] stream cost %v, stream maybe valid", tagName, cost)
				}
				parserWG.Done()
			}()
			err := aitag.Parse(
				utils.UTF8Reader(pReader),
				aitag.WithCallback(tagName, m.nonce, func(rd io.Reader) {
					var out bytes.Buffer
					writer := mirrorPipe(fieldName) // if the fieldName which this tag maps to has field stream handler, create pipe writer
					if writer != nil {
						defer writer.Close()
						rd = io.TeeReader(rd, writer)
					}
					_, err := io.Copy(&out, rd)
					if err != nil {
						log.Errorf("Failed to read data for tag %s: %v", tagName, err)
						return
					}
					action.ForceSet(fieldName, out.String()) // set the tag content to action param, tag content is primary over field stream handler
				}))
			if err != nil && err != io.EOF {
				log.Warnf("Failed to read tag %s: %v", tagName, err)
			}
		}
		tagsParseHandles = append(tagsParseHandles, handle)
	}
	// mirror stream for tag parsing
	reader = utils.CreateUTF8StreamMirror(reader, tagsParseHandles...)
	onReaderFinished := m.onReaderFinished
	go func() { // main goroutine to extract json
		defer func() {
			for _, onFinished := range onReaderFinished {
				onFinished()
			}
		}()
		actionStart := utils.NewBool(false) // indicate whether action is started
		setStart := func(hitName string) {
			action.SetName(hitName)
			action.Set(ActionMagicKey, hitName)
			actionStart.Set()
		}

		opts := m.jsonCallback

		//  stream set field handler
		opts = append(opts, jsonextractor.WithFormatKeyValueCallback(func(key, data any, parents []string) {
			if key == "" {
				return
			}
			if actionStart.IsSet() {
				keyString := utils.InterfaceToString(key)

				if len(parents) > 0 { // build full key with parents
					fullKeyString := strings.Join(append(parents, keyString), ".")
					action.Set(fullKeyString, data) // set full key param
				}
				action.Set(keyString, data) // verbose save with simple key, legacy support
				return
			}
			if utils.InterfaceToString(key) == "@action" {
				value := utils.InterfaceToString(data)
				if utils.StringArrayContains(actionNames, value) {
					setStart(value)
				} else if mapData, ok := data.(map[string]any); ok {
					for _, v := range mapData {
						if utils.StringArrayContains(actionNames, utils.InterfaceToString(v)) {
							value = utils.InterfaceToString(v)
							setStart(value)
							return
						}
					}
				}
			}
		}))

		fixParams := func(generalParams map[string]any) {
			action.Set(generalParamsKey, generalParams)
			for k, v := range generalParams {
				action.Set(k, v)
			}
		}

		opts = append(opts, jsonextractor.WithObjectCallback(func(data map[string]any) { // set the general object if @action matched
			dataParams := aitool.InvokeParams(data)
			if !dataParams.Has("@action") {
				return
			}
			targetString := dataParams.GetString("@action")
			if targetString != "" {
				if utils.StringArrayContains(actionNames, targetString) {
					fixParams(data)
					return
				}
			} else {
				target := dataParams.GetObject("@action")
				for _, v := range target {
					if utils.StringArrayContains(actionNames, utils.InterfaceToString(v)) {
						fixParams(data)
						return
					}
				}
			}
		}))

		for name, _ := range fieldHandlerMap { // register field stream handlers for json field type
			opts = append(opts, jsonextractor.WithRegisterFieldStreamHandlerAndStartCallback(name, nil, func(key string, reader io.Reader, parents []string) { // sync create writer
				w := mirrorPipe(key)
				go func() {
					defer w.Close()
					_, err := io.Copy(w, reader)
					if err != nil {
						log.Errorf("Failed to write field stream for key %s: %v", key, err)
					}
				}()
			}))
		}

		var buf bytes.Buffer
		err := jsonextractor.ExtractStructuredJSONFromStream(io.TeeReader(reader, &buf), opts...) // extract json from stream
		if err != nil {
			log.Errorf("Failed to extract action from stream: %v, buffer: %s", err, buf.String())

		}

		parserWG.Wait() // wait tag parsers finished
		parseFinish()   // signal parse finish

		streamWg.Wait() // wait all stream handlers finished
		streamFinish()  // signal stream finish
	}()

	return action
}

func NewActionMaker(actionName string, opts ...ActionMakerOption) *ActionMaker {
	maker := &ActionMaker{
		actionName: actionName,
	}
	for _, opt := range opts {
		opt(maker)
	}
	return maker
}

func ExtractActionFromStream(ctx context.Context, reader io.Reader, actionName string, opts ...ActionMakerOption) (*Action, error) {
	maker := NewActionMaker(actionName, opts...)
	action := maker.ReadFromReader(ctx, reader)
	return action, nil
}

func ExtractValidActionFromStream(ctx context.Context, reader io.Reader, actionName string, opts ...ActionMakerOption) (*Action, error) {
	maker := NewActionMaker(actionName, opts...)
	action := maker.ReadFromReader(ctx, reader)
	action.WaitParse(ctx)
	action.WaitStream(ctx)
	if !action.ValidCheck(append(maker.alias, actionName)...) {
		return nil, utils.Errorf("action @action not found or invalid, expect one of: %v", append(maker.alias, actionName))
	}
	return action, nil
}

// ExtractAction 从字符串中提取指定的 Action 对象，支持别名，这里隐含一个强校验行为，即会等待处理完毕之后检查是否有可用的Action
func ExtractAction(i string, actionName string, alias ...string) (*Action, error) {
	action, err := ExtractActionFromStream(context.Background(), strings.NewReader(i), actionName, WithActionAlias(alias...))
	if err != nil {
		return nil, err
	}
	action.WaitParse(context.Background())
	action.WaitStream(context.Background())
	if !action.ValidCheck(append(alias, actionName)...) {
		return nil, utils.Errorf("action @action not found or invalid, expect one of: %v", append(alias, actionName))
	}
	return action, nil
}

func ExtractAllAction(i string) []*Action {
	var acs []*Action
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, pairs := range jsonextractor.ExtractObjectIndexes(i) {
		ac := &Action{
			params:       make(map[string]any),
			barrier:      utils.NewCondBarrierContext(ctx),
			parseFinish:  ctx,
			streamFinish: ctx,
		}
		start, end := pairs[0], pairs[1]
		jsonRaw := i[start:end]
		var i = make(map[string]any)
		err := json.Unmarshal([]byte(jsonRaw), &i)
		if err != nil {
			continue
		}
		if rawData, ok := i["@action"]; ok && fmt.Sprint(rawData) != "" {
			action := fmt.Sprint(rawData)
			ac.name = action
			ac.params = i
			if ac.params == nil {
				ac.params = make(map[string]any)
			}
			acs = append(acs, ac)
		}
	}
	return acs
}

func NewSimpleAction(name string, params aitool.InvokeParams) *Action {
	if params == nil {
		params = make(aitool.InvokeParams)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return &Action{
		name:         name,
		params:       params,
		barrier:      utils.NewCondBarrierContext(ctx),
		streamFinish: ctx,
		parseFinish:  ctx,
	}
}
