package yaklib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/bcicen/jstream"
	"github.com/jinzhu/gorm/dialects/postgres"
	"github.com/pkg/errors"
	"github.com/vjeantet/grok"
	"github.com/yaklang/yaklang/common/log"
)

var (
	grokParser *grok.Grok
)

var GrokExports = map[string]interface{}{
	"ExtractIPv4":     RegexpMatchIPv4,
	"ExtractIPv6":     RegexpMatchIPv6,
	"ExtractIP":       RegexpMatchIP,
	"ExtractEmail":    RegexpMatchEmail,
	"ExtractPath":     RegexpMatchPathParam,
	"ExtractTTY":      RegexpMatchTTY,
	"ExtractURL":      RegexpMatchURL,
	"ExtractHostPort": RegexpMatchHostPort,
	"ExtractMac":      RegexpMatchMac,
}

func init() {
	if grokParser != nil {
		return
	}
	var err error
	grokParser, err = getGrokParser()
	if err != nil {
		panic("BUG: get grok parser failed: " + err.Error())
	}
}

func getGrokParser() (*grok.Grok, error) {
	parser, err := grok.NewWithConfig(&grok.Config{
		NamedCapturesOnly:   false,
		SkipDefaultPatterns: false,
		RemoveEmptyValues:   true,
	})
	if err != nil {
		return nil, err
	}

	err = parser.AddPatternsFromMap(map[string]string{
		`COMMONVERSION`: `(%{INT}\.?)+[a-zA-Z]*?`,
	})
	if err != nil {
		return nil, err
		//panic(fmt.Sprintf("add grok pattern failed: %s", err))
	}

	return parser, nil
}

type GrokResult map[string][]string

func (g GrokResult) Get(key string) string {
	res := g.GetAll(key)
	if len(res) > 0 {
		return res[0]
	}
	return ""
}

func (g GrokResult) GetAll(key string) []string {
	if g == nil {
		return nil
	}

	res, ok := g[key]
	if !ok {
		return nil
	}

	if res == nil {
		return nil
	}

	return res
}

func (g GrokResult) GetOr(key string, value string) string {
	if g.Get(key) == "" {
		return value
	}
	return g.Get(key)
}

// Grok 用于将字符串 line 使用 Grok 以规则 rule 进行解析，并返回解析结果(map)，参考 https://doc.yonyoucloud.com/doc/logstash-best-practice-cn/filter/grok.html 获取更多信息。
// Example:
// ```
// str.Grok("04/18-00:59:45.385191", "%{MONTHNUM:month}/%{MONTHDAY:day}-%{TIME:time}") // map[HOUR:[00] MINUTE:[59] SECOND:[45.385191] day:[18] month:[04] time:[00:59:45.385191]]
// ```
func Grok(line string, rule string) GrokResult {
	results, err := grokParser.ParseToMultiMap(rule, line)
	if err != nil {
		return nil
	}
	return results
}

func GrokWithMultiPattern(line string, rule string, p map[string]string) GrokResult {
	par, err := getGrokParser()
	if err != nil {
		return nil
	}

	err = par.AddPatternsFromMap(p)
	if err != nil {
		log.Errorf("add pattern failed: %s", err)
		return nil
	}

	res, err := par.ParseToMultiMap(rule, line)
	if err != nil {
		log.Errorf("parse [%s] failed; %s", line, err)
		return nil
	}
	return res
}

func JsonStreamToMapListWithDepth(reader io.Reader, i int) []map[string]interface{} {
	if reader == nil {
		log.Error("jstream get empty reader...")
		return nil
	}

	var r []map[string]interface{}
	for kv := range jstream.NewDecoder(reader, i).Stream() {
		m := make(map[string]interface{})
		switch raw := kv.Value.(type) {
		case map[string]interface{}:
			if raw == nil {
				continue
			}
			for k, v := range raw {
				m[k] = v
			}
		default:
			log.Errorf("recv: %v cannot handled", kv.Value)
			continue
		}
		if len(m) > 0 {
			r = append(r, m)
		}
	}
	return r
}

// JsonStreamToMapList 从 reader 中读取字符串并解析为 map 列表
// Example:
// ```
// reader, writer = io.Pipe()
//
//	go fn {
//	    writer.Write(`{"a":1,"b":2} {"c":3, "d":4}`)
//	    writer.Close()
//	}
//
// println(str.JsonStreamToMapList(reader)) // [map[a:1 b:2] map[c:3 d:4]]
// ```
func JsonStreamToMapList(reader io.Reader) []map[string]interface{} {
	return JsonStreamToMapListWithDepth(reader, 0)
}

// JsonToMapList 将 json 字符串 line 解析为 map 列表
// Example:
// ```
// str.JsonToMapList(`{"a":1,"b":2} {"c":3, "d":4}`) // [map[a:1 b:2] map[c:3 d:4]]
// ```
func JsonToMapList(line string) []map[string]string {
	var r []map[string]string
	for kv := range jstream.NewDecoder(bytes.NewBufferString(line), 0).Stream() {
		m := map[string]string{}
		switch raw := kv.Value.(type) {
		case map[string]interface{}:
			for k, v := range raw {
				m[k] = fmt.Sprintf("%v", v)
			}
		default:
			log.Errorf("recv: %v cannot handled", kv.Value)
			continue
		}
		if len(m) > 0 {
			r = append(r, m)
		}
	}
	return r
}

// JsonToMap 将 json 字符串 line 解析为 map
// Example:
// ```
// str.JsonToMap(`{"a":1,"b":2}`) // map[a:1 b:2]
// ```
func JsonToMap(line string) map[string]string {
	raws := JsonToMapList(line)
	if len(raws) > 0 {
		return raws[0]
	}
	return nil
}

// ParamsGetOr 从 map 中获取 key 对应的值，如果不存在则返回 defaultValue
// Example:
// ```
// str.ParamsGetOr({"a": "1"}, "a", "2") // 1
// str.ParamsGetOr({"a": "1"}, "b", "2") // 2
// ```
func ParamsGetOr(i map[string]string, key, defaultValue string) string {
	if i != nil {
		raw, ok := i[key]
		if ok {
			return raw
		}
	}
	return defaultValue
}

func JsonRawByteToMap(jbyte json.RawMessage) (map[string]interface{}, error) {
	var res map[string]interface{}
	err := json.Unmarshal(jbyte, &res)
	return res, err
}
func JsonbToMap(jb postgres.Jsonb) (map[string]interface{}, error) {
	if jb.RawMessage == nil {
		return nil, errors.Errorf("content is nil")
	}
	var res map[string]interface{}
	err := json.Unmarshal(jb.RawMessage, &res)
	return res, err
}
func JsonbToString(jb postgres.Jsonb) (string, error) {
	if jb.RawMessage == nil {
		return "", errors.Errorf("content is nil")
	}
	var res string
	err := json.Unmarshal(jb.RawMessage, &res)
	return res, err
}

func JsonStrToVarList(jstr string) ([]interface{}, error) {
	var res []interface{}
	err := json.Unmarshal([]byte(jstr), &res)
	return res, err
}
func JsonbToVarList(jb postgres.Jsonb) ([]interface{}, error) {
	var res []interface{}
	err := json.Unmarshal(jb.RawMessage, &res)
	return res, err
}
