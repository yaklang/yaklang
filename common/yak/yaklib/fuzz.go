package yaklib

import (
	"bytes"
	"container/list"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protowire"
	"gopkg.in/yaml.v3"
)

var EmptyBytesSlice = make([][]byte, 0)

// utils
func strVisible(key string) bool {
	for _, c := range key {
		if c != '\n' && c != '\r' && !strconv.IsPrint(c) {
			return false
		}
	}
	return true
}

// fuzz
func _fuzz(i interface{}) []string {
	res := mutate.InterfaceToFuzzResults(i)
	if res != nil {
		return res
	}
	return []string{fmt.Sprint(i)}
}

func _fuzzFuncEx(i interface{}, i2 interface{}) []string {
	res, _ := mutate.QuickMutate(
		utils.InterfaceToString(i),
		consts.GetGormProfileDatabase(), mutate.MutateWithExtraParams(
			utils.InterfaceToMap(i2),
		),
	)
	if len(res) > 0 {
		return res
	}
	return []string{utils.InterfaceToString(i)}
}

func _urlToFuzzRequest(method string, i interface{}) (*mutate.FuzzHTTPRequest, error) {
	https, reqBytes, err := lowhttp.ParseUrlToHttpRequestRaw(method, i)
	if err != nil {
		return nil, err
	}
	return mutate.NewFuzzHTTPRequest(reqBytes, mutate.OptHTTPS(https))
}

func _fuzzFunc(i interface{}, cb func(i *mutate.MutateResult), params ...interface{}) error {
	var res = make(map[string][]string)
	for _, i := range params {
		if i == nil {
			continue
		}
		for k, v := range utils.InterfaceToMap(i) {
			res[k] = append(res[k], v...)
		}
	}
	_, err := mutate.QuickMutateWithCallbackEx(
		utils.InterfaceToString(i),
		consts.GetGormProfileDatabase(),
		[]func(i *mutate.MutateResult){cb},
		mutate.MutateWithExtraParams(utils.InterfaceToMap(res)),
	)
	if err != nil {
		log.Errorf("mutate error: %s", err)
		return err
	}
	return nil
}

// protobuf
type ProtobufRecord struct {
	Index protowire.Number `json:"index" yaml:"index"`
	Type  string           `json:"type" yaml:"type"`
	Value interface{}      `json:"value,omitempty" yaml:"value,omitempty,flow"`
}

type _ProtobufRecord struct {
	Index protowire.Number `json:"index" yaml:"index"`
	Type  string           `json:"type" yaml:"type"`
	Value yaml.Node        `json:"value,omitempty" yaml:"value,omitempty,flow"`
}

func newProtobufRecord(index protowire.Number, typ string, value interface{}) *ProtobufRecord {
	return &ProtobufRecord{
		Index: index,
		Type:  typ,
		Value: value,
	}
}

func (r *ProtobufRecord) String() string {
	if r.Type == "group" {
		return fmt.Sprintf("%d: (", r.Index)
	} else if r.Type == "endgroup" {
		return ")"
	} else if r.Type == "string" {
		return fmt.Sprintf("%d: %s: %#v", r.Index, r.Type, r.Value)
	}
	return fmt.Sprintf("%d: %s: %v", r.Index, r.Type, r.Value)
}

func (r *ProtobufRecord) ToBytes() []byte {
	var b []byte
	switch r.Type {
	case "varint":
		b = protowire.AppendTag(b, r.Index, protowire.VarintType)
		b = protowire.AppendVarint(b, r.Value.(uint64))
	case "fixed32":
		b = protowire.AppendTag(b, r.Index, protowire.Fixed32Type)
		b = protowire.AppendFixed32(b, r.Value.(uint32))
	case "fixed64":
		b = protowire.AppendTag(b, r.Index, protowire.Fixed64Type)
		b = protowire.AppendFixed64(b, r.Value.(uint64))
	case "string":
		b = protowire.AppendTag(b, r.Index, protowire.BytesType)
		b = protowire.AppendBytes(b, []byte(r.Value.(string)))
	case "bytes":
		b = protowire.AppendTag(b, r.Index, protowire.BytesType)
		b = protowire.AppendBytes(b, r.Value.([]byte))
	case "group":
		b = protowire.AppendTag(b, r.Index, protowire.StartGroupType)
	case "endgroup":
		b = protowire.AppendTag(b, r.Index, protowire.EndGroupType)
	}
	return b
}

type ProtobufRecords struct {
	Records []*ProtobufRecord
	err     error `json:"-" yaml:"-"`
}

func newProtobufRecords() *ProtobufRecords {
	return &ProtobufRecords{
		Records: make([]*ProtobufRecord, 0),
	}
}

// utils
func (r *ProtobufRecords) Find(index int) []*ProtobufRecord {
	records := make([]*ProtobufRecord, 0)
	for _, record := range r.Records {
		if int(record.Index) == index {
			records = append(records, record)
		}
	}
	return records
}

func (r *ProtobufRecords) Error() error {
	return r.err
}

// marshal / unmarshal
func (r *ProtobufRecords) MarshalJSON() ([]byte, error) {
	newRecords := make([]*ProtobufRecord, 0, len(r.Records))
	for _, record := range r.Records {
		if record.Type == "endgroup" {
			continue
		}
		newRecords = append(newRecords, record)
	}
	return json.Marshal(newRecords)
}

func (r *ProtobufRecords) UnmarshalJSON(data []byte) error {
	var (
		records    []*ProtobufRecord
		recordList = list.New()
	)
	err := json.Unmarshal(data, &records)
	if err != nil {
		return err
	}

	for _, record := range records {
		if record.Type == "group" { // add endgroup
			recordList.PushFront(newProtobufRecord(record.Index, "endgroup", nil))
		} else if record.Type == "bytes" { // recover bytes
			if bytesString, ok := record.Value.(string); ok {
				bytes, err := base64.StdEncoding.DecodeString(bytesString)
				if err != nil {
					return err
				}
				record.Value = bytes
			}
		} else if record.Type == "varint" || record.Type == "fixed64" {
			record.Value = uint64(record.Value.(float64))
		} else if record.Type == "fixed32" {
			record.Value = uint32(record.Value.(float64))
		}
	}

	// add endgroups
	for e := recordList.Front(); e != nil; e = e.Next() {
		records = append(records, e.Value.(*ProtobufRecord))
	}

	r.Records = records
	return nil
}

func (r *ProtobufRecords) MarshalYAML() (interface{}, error) {
	newRecords := make([]*ProtobufRecord, 0, len(r.Records))
	for _, record := range r.Records {
		if record.Type == "endgroup" {
			continue
		}
		newRecords = append(newRecords, record)
	}
	return newRecords, nil
}

func (r *ProtobufRecords) UnmarshalYAML(node *yaml.Node) error {
	var (
		records    []*_ProtobufRecord
		newrecords []*ProtobufRecord
		recordList = list.New()
	)

	if err := node.Decode(&records); err != nil {
		return err
	}
	newrecords = make([]*ProtobufRecord, len(records))

	for i, record := range records {
		newrecords[i] = new(ProtobufRecord)
		newrecords[i].Index = record.Index
		newrecords[i].Type = record.Type
		switch record.Type {
		case "group":
			recordList.PushFront(newProtobufRecord(record.Index, "endgroup", nil))
			newrecords[i].Value = nil
		case "string":
			newrecords[i].Value = new(string)
		case "bytes":
			newrecords[i].Value = new([]byte)
		case "varint":
			fallthrough
		case "fixed64":
			newrecords[i].Value = new(uint64)
		case "fixed32":
			newrecords[i].Value = new(uint32)
		}
		if record.Type == "group" {
			continue
		}

		if err := record.Value.Decode(newrecords[i].Value); err != nil {
			return err
		}

		v := reflect.ValueOf(newrecords[i].Value)
		switch record.Type {
		case "varint":
			fallthrough
		case "fixed64":
			newrecords[i].Value = v.Elem().Uint()
		case "fixed32":
			newrecords[i].Value = uint32(v.Elem().Uint())
		case "string":
			newrecords[i].Value = v.Elem().String()
		case "bytes":
			newrecords[i].Value = v.Elem().Bytes()
		}

	}

	r.Records = newrecords
	return nil
}

// protobuf convert
func (r *ProtobufRecords) String() string {
	var (
		builder strings.Builder
		inGroup int = 0
	)
	if r == nil {
		return ""
	}

	for _, record := range r.Records {
		if record.Type == "group" {
			inGroup += 1
		} else if record.Type == "endgroup" {
			inGroup -= 1
		}
		builder.WriteString(record.String())
		if inGroup <= 0 {
			builder.WriteRune('\n')
		} else if record.Type != "group" && record.Type != "endgroup" {
			builder.WriteRune(',')
		} else {
			builder.WriteRune(' ')
		}
	}
	return strings.TrimSpace(builder.String())
}

func (r *ProtobufRecords) ToJSON() string {
	if r == nil {
		return ""
	}
	if bytes, err := json.MarshalIndent(r, "", "  "); err != nil {
		return ""
	} else {
		return string(bytes)
	}
}

func (r *ProtobufRecords) ToYAML() string {
	if r == nil {
		return ""
	}
	if bytes, err := yaml.Marshal(r); err != nil {
		return ""
	} else {
		return string(bytes)
	}
}

func (r *ProtobufRecords) ToBytes() []byte {
	var buf bytes.Buffer

	if r == nil {
		return nil
	}

	for _, record := range r.Records {
		buf.Write(record.ToBytes())
	}
	return buf.Bytes()
}

func (r *ProtobufRecords) ToHex() string {
	if r == nil {
		return ""
	}
	return hex.EncodeToString(r.ToBytes())
}

// protobuf fuzz

func (r *ProtobufRecords) fuzzRecord(record *ProtobufRecord, callback func(index int, typ string, data interface{}) interface{}) ([][]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("records is nil")
	} else if record.Type == "group" || record.Type == "endgroup" {
		return EmptyBytesSlice, nil
	}

	oldRecordValue := record.Value
	defer func() {
		record.Value = oldRecordValue
	}()

	value := callback(int(record.Index), record.Type, record.Value)
	if value == nil {
		return EmptyBytesSlice, nil
	}
	valueSlice := utils.InterfaceToBytesSlice(value)

	result := make([][]byte, 0, len(valueSlice))

	switch record.Type {
	case "varint":
		fallthrough
	case "fixed64":
		fallthrough
	case "fixed32":
		valueIntSlice := make([]int, 0, len(valueSlice))
		for _, v := range valueSlice {
			if i, err := strconv.Atoi(string(v)); err != nil {
				return nil, errors.Wrapf(err, "invalid int: %#v", v)
			} else {
				valueIntSlice = append(valueIntSlice, i)
			}
		}

		for _, v := range valueIntSlice {
			if record.Type == "varint" || record.Type == "fixed64" {
				record.Value = uint64(v)
			} else {
				record.Value = uint32(v)
			}
			result = append(result, r.ToBytes())
		}

		return result, nil

	case "bytes":
		fallthrough
	case "string":
		for _, v := range valueSlice {
			if record.Type == "string" {
				record.Value = string(v)
			} else {
				record.Value = v
			}
			result = append(result, r.ToBytes())
		}
		return result, nil
	}

	return nil, fmt.Errorf("invalid record type: %s", record.Type)
}

func (r *ProtobufRecords) FuzzIndex(index int, callback func(index int, typ string, data interface{}) interface{}) ([][]byte, error) {
	var (
		err         error
		tempResults [][]byte
		results     = make([][]byte, 0)
	)

	if r == nil {
		return nil, fmt.Errorf("records is nil")
	} else if r.err != nil {
		return nil, r.err
	}

	records := r.Find(index)
	if len(records) == 0 {
		return nil, fmt.Errorf("Cannot find record with index %d", index)
	}

	for _, record := range r.Records {
		if tempResults, err = r.fuzzRecord(record, callback); err != nil {
			return nil, err
		}
		results = append(results, tempResults...)
	}

	return results, nil
}

func (r *ProtobufRecords) FuzzEveryIndex(callback func(index int, typ string, data interface{}) interface{}) ([][]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("records is nil")
	} else if r.err != nil {
		return nil, r.err
	}

	var (
		err         error
		tempResults [][]byte
		results     = make([][]byte, 0)
	)
	for _, record := range r.Records {
		if tempResults, err = r.fuzzRecord(record, callback); err != nil {
			return nil, err
		}
		results = append(results, tempResults...)
	}

	return results, nil
}

// protobuf export
func _protobufRecordFromBytes(b []byte) ([]byte, []*ProtobufRecord, error) {
	var (
		newRecords []*ProtobufRecord
		err        error

		records = make([]*ProtobufRecord, 0)
	)

	index, typ, n := protowire.ConsumeTag(b)
	// fmt.Printf("debug index:%#v type:%#v n:%d\n", index, typ, n)
	if index < 0 || typ < 0 || n < 0 {
		return nil, nil, fmt.Errorf("cunsume protobuf tag error")
	}
	b = b[n:]
	switch typ {
	case protowire.VarintType:
		v, m := protowire.ConsumeVarint(b)
		records = append(records, newProtobufRecord(index, "varint", v))
		b = b[m:]
	case protowire.Fixed32Type:
		v, m := protowire.ConsumeFixed32(b)
		records = append(records, newProtobufRecord(index, "fixed32", v))
		b = b[m:]
	case protowire.Fixed64Type:
		v, m := protowire.ConsumeFixed64(b)
		records = append(records, newProtobufRecord(index, "fixed64", v))
		b = b[m:]
	case protowire.BytesType:
		v, m := protowire.ConsumeBytes(b)
		if strVisible(string(v)) {
			records = append(records, newProtobufRecord(index, "string", string(v)))
		} else {
			records = append(records, newProtobufRecord(index, "bytes", v))
		}
		b = b[m:]
	case protowire.StartGroupType:
		records = append(records, newProtobufRecord(index, "group", nil))
		b, newRecords, err = _protobufRecordFromBytes(b)
		if err != nil {
			return nil, nil, err
		}
		records = append(records, newRecords...)
	case protowire.EndGroupType:
		records = append(records, newProtobufRecord(index, "endgroup", nil))
	default:
		return nil, nil, fmt.Errorf("Unknown protobuf type: %d", typ)
	}
	return b, records, nil
}

func _protobufRecordsFromBytes(i interface{}) *ProtobufRecords {
	var (
		b = utils.InterfaceToBytes(i)

		newRecords = newProtobufRecords()

		records []*ProtobufRecord
		err     error
	)

	for {
		b, records, err = _protobufRecordFromBytes(b)
		if err != nil {
			newRecords.err = err
			break
		}
		newRecords.Records = append(newRecords.Records, records...)
		if len(b) <= 0 {
			break
		}
	}
	return newRecords
}

func _protobufRecordsFromHex(i interface{}) *ProtobufRecords {
	var records *ProtobufRecords

	s := utils.InterfaceToString(i)
	b, err := hex.DecodeString(s)
	if err != nil {
		records = newProtobufRecords()
		records.err = errors.Wrapf(err, "hex decode error")
	} else {
		records = _protobufRecordsFromBytes(b)
	}

	return records
}

func _protobufRecordsFromJSON(i interface{}) *ProtobufRecords {
	records := newProtobufRecords()
	b := utils.InterfaceToBytes(i)
	err := json.Unmarshal(b, records)
	if err != nil {
		records.err = errors.Wrapf(err, "json unmarshal error")
	}
	return records
}

func _protobufRecordsFromYAML(i interface{}) *ProtobufRecords {
	records := newProtobufRecords()
	b := utils.InterfaceToBytes(i)
	err := yaml.Unmarshal(b, records)
	if err != nil {
		records.err = errors.Wrapf(err, "yaml unmarshal error")
	}
	return records
}

var FuzzExports = map[string]interface{}{
	"Strings":            _fuzz,
	"StringsWithParam":   _fuzzFuncEx,
	"StringsFunc":        _fuzzFunc,
	"HTTPRequest":        mutate.NewFuzzHTTPRequest,
	"MustHTTPRequest":    mutate.NewMustFuzzHTTPRequest,
	"https":              mutate.WithPoolOpt_Https,
	"proxy":              mutate.WithPoolOpt_Proxy,
	"context":            mutate.WithPoolOpt_Context,
	"noEncode":           mutate.OptDisableAutoEncode,
	"showTag":            mutate.OptFriendlyDisplay,
	"UrlsToHTTPRequests": mutate.UrlsToHTTPRequests,
	"UrlToHTTPRequest":   _urlToFuzzRequest,

	// protobuf fuzz
	"ProtobufHex":   _protobufRecordsFromHex,
	"ProtobufBytes": _protobufRecordsFromBytes,
	"ProtobufJSON":  _protobufRecordsFromJSON,
	"ProtobufYAML":  _protobufRecordsFromYAML,

	"WithDelay":           mutate.WithPoolOPt_DelaySeconds,
	"WithNamingContext":   mutate.WithPoolOpt_NamingContext,
	"WithConcurrentLimit": mutate.WithPoolOpt_Concurrent,
	"WithTimeOut":         mutate.WithPoolOpt_Timeout,
}
