package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"gopkg.in/yaml.v2"
	"net"
	"reflect"
	"strconv"
	"strings"
)

func getSubData(d any, key string) (any, bool) {
	p := strings.Split(key, "/")
	for _, ele := range p {
		refV := reflect.ValueOf(d)
		if refV.Kind() == reflect.Map {
			v := refV.MapIndex(reflect.ValueOf(ele))
			if !v.IsValid() {
				return nil, false
			} else {
				d = v.Interface()
			}
		} else if refV.Kind() == reflect.Slice || refV.Kind() == reflect.Array {
			if !strings.HasPrefix(ele, "#") {
				return nil, false
			}
			index, err := strconv.Atoi(ele[1:])
			if err != nil {
				return nil, false
			}
			if index >= refV.Len() {
				return nil, false
			}
			d = refV.Index(index).Interface()
		} else {
			return nil, false
		}
	}
	return d, true
}
func GetSubNode(node *base.Node, path string) *base.Node {
	splits := strings.Split(path, "/")
	if len(splits) == 0 {
		return node
	}
	var getSubNode func(node *base.Node, path []string) *base.Node
	getSubNode = func(node *base.Node, path []string) *base.Node {
		if len(path) == 0 {
			return node
		}

		for _, sub := range stream_parser.GetSubNodes(node) {
			if sub.Name == path[0] {
				return getSubNode(sub, path[1:])
			}
		}
		return nil
	}
	return getSubNode(node, splits)
}
func NodeToMap(node *base.Node) any {
	if node.Cfg.Has(stream_parser.CfgNodeResult) {
		return stream_parser.GetResultByNode(node)
	}
	if node.Cfg.GetBool(stream_parser.CfgIsList) {
		res := []any{}
		for _, sub := range node.Children {
			d := NodeToMap(sub)
			if d != nil {
				res = append(res, d)
			}
		}
		if len(res) == 0 {
			return nil
		}
		return res
	} else {
		res := map[string]any{}
		for _, sub := range node.Children {
			d := NodeToMap(sub)
			if d != nil {
				res[sub.Name] = NodeToMap(sub)
			}
		}
		if len(res) == 0 {
			return nil
		}
		return res
	}
}
func NodeToBytes(node *base.Node) []byte {
	buffer := node.Ctx.GetItem("buffer").(*bytes.Buffer)
	return buffer.Bytes()
	//res := []byte{}
	//var toBytes func(nodeRes *base.Node)
	//toBytes = func(node *base.Node) {
	//	if stream_parser.NodeHasResult(node) {
	//		res = append(res, stream_parser.GetBytesByNode(node)...)
	//	} else {
	//		for _, sub := range node.Children {
	//			toBytes(sub)
	//		}
	//	}
	//}
	//toBytes(node)
	//return res
}
func DumpNode(node *base.Node) {
	println(nodeResultToYaml(node))
}
func SdumpNode(node *base.Node) string {
	return nodeResultToYaml(node)
}

func nodeResultToYaml(node *base.Node) (result string) {
	var toMap func(nodeRes *base.Node) any
	_ = toMap

	toMap = func(node *base.Node) any {
		if stream_parser.NodeHasResult(node) {
			data := stream_parser.GetResultByNode(node)
			if v, ok := data.([]byte); ok {
				data = fmt.Sprintf("%x", v)
			}
			return data
		} else {
			res := yaml.MapSlice{}
			for _, sub := range node.Children {
				res = append(res, yaml.MapItem{
					Key:   sub.Name,
					Value: toMap(sub),
				})
			}
			return res
		}
	}
	//nodeRes := node.Cfg.GetItem(stream_parser.CfgNodeResult).(*stream_parser.NodeResult)
	res, err := yaml.Marshal(toMap(node))
	if err != nil {
		log.Errorf("error when marshal node to yaml: %v", err)
	}
	return string(res)
}
func ToUint64(d any) (uint64, error) {
	switch ret := d.(type) {
	case uint64:
		return ret, nil
	case uint32:
		return uint64(ret), nil
	case uint16:
		return uint64(ret), nil
	case uint8:
		return uint64(ret), nil
	case int64:
		return uint64(ret), nil
	case int32:
		return uint64(ret), nil
	case int16:
		return uint64(ret), nil
	case int8:
		return uint64(ret), nil
	case int:
		return uint64(ret), nil
	default:
		return 0, fmt.Errorf("unexpected type: %v", reflect.TypeOf(d))
	}
}
func ResultToJson(d any) (string, error) {
	var toRawData func(d any) any
	toRawData = func(d any) any {
		refV := reflect.ValueOf(d)
		switch ret := d.(type) {
		case []uint8:
			return string(ret)
		}
		if !refV.CanAddr() {
			return d
		}
		if refV.Kind() == reflect.Slice || refV.Kind() == reflect.Array {
			for i := 0; i < refV.Len(); i++ {
				refV.Index(i).Set(reflect.ValueOf(toRawData(refV.Index(i).Interface())))
			}
			return refV.Interface()
		} else if refV.Kind() == reflect.Map {
			for _, k := range refV.MapKeys() {
				refV.SetMapIndex(k, reflect.ValueOf(toRawData(refV.MapIndex(k).Interface())))
			}
			return refV.Interface()
		} else {
			return d
		}
	}
	rawData := toRawData(d)
	res, err := json.Marshal(rawData)
	if err != nil {
		return "", err
	}
	return string(res), nil
}
func JsonToResult(jsonStr string) (any, error) {
	d := map[string]any{}
	err := json.Unmarshal([]byte(jsonStr), &d)
	if err != nil {
		return nil, err
	}
	var toRawDataErr error
	var toRawData func(d any) any
	toRawData = func(d any) any {
		refV := reflect.ValueOf(d)
		if refV.Kind() == reflect.Slice || refV.Kind() == reflect.Array {
			for i := 0; i < refV.Len(); i++ {
				refV.Index(i).Set(reflect.ValueOf(toRawData(refV.Index(i).Interface())))
			}
			return refV.Interface()
		} else if refV.Kind() == reflect.Map {
			if len(refV.MapKeys()) == 1 {
				refKey := refV.MapKeys()[0]
				if v, ok := refKey.Interface().(string); ok && v == "__data__" {
					if v, ok = refV.MapIndex(refKey).Interface().(string); ok {
						res, err := codec.DecodeBase64(v)
						if err != nil {
							toRawDataErr = err
						}
						return res
					}
				}
			}
			for _, k := range refV.MapKeys() {
				refV.SetMapIndex(k, reflect.ValueOf(toRawData(refV.MapIndex(k).Interface())))
			}
			return refV.Interface()
		} else {
			return d
		}
	}
	rawData := toRawData(d)
	if toRawDataErr != nil {
		return nil, toRawDataErr
	}
	return rawData, nil
}
func DumpNodeValueYaml(d *base.NodeValue) (string, error) {
	var toRawData func(d any) any
	toRawData = func(d any) any {
		switch d.(type) {
		case []byte:
			return codec.EncodeToHex(d)
		case []*base.NodeValue:
			nodeValue := d.([]*base.NodeValue)
			res := yaml.MapSlice{}
			for i := 0; i < len(nodeValue); i++ {
				d := nodeValue[i]
				res = append(res, toRawData(d).(yaml.MapItem))
			}
			return res
		case *base.NodeValue:
			d := d.(*base.NodeValue)
			name := d.Name
			return yaml.MapItem{
				Key:   name,
				Value: toRawData(d.Value),
			}
		default:
			return d
		}
	}
	rawData := toRawData(d)
	res, err := yaml.Marshal(rawData)
	if err != nil {
		return "", err
	}
	return string(res), nil
}
func ResultToYaml(d any) (string, error) {
	var toRawData func(d any) any
	toRawData = func(d any) any {
		if v, ok := d.([]byte); ok {
			return codec.EncodeToHex(v)
		}
		refV := reflect.ValueOf(d)
		if refV.Kind() == reflect.Slice || refV.Kind() == reflect.Array {
			res := yaml.MapSlice{}
			for i := 0; i < refV.Len(); i++ {
				d := refV.Index(i).Interface().(map[string]any)
				name := d["name"]
				val := d["value"]
				res = append(res, yaml.MapItem{
					Key:   name,
					Value: toRawData(val),
				})
			}
			return res
		} else {
			return d
		}
	}
	rawData := toRawData(d)
	res, err := yaml.Marshal(rawData)
	if err != nil {
		return "", err
	}
	return string(res), nil
}
func checksum(data []byte) uint16 {
	var sum uint32

	// 按16位进行累加
	for i := 0; i < len(data)-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i:]))
	}

	// 如果字节数组是奇数，将最后一个字节加入计算
	if len(data)%2 == 1 {
		sum += uint32(data[len(data)-1]) << 8
	}

	// 将进位加到校验和上并再次加上进位
	sum = (sum >> 16) + (sum & 0xFFFF)
	sum += (sum >> 16)

	// 取反
	var checksum uint16 = ^uint16(sum & 0xFFFF)

	return checksum
}

// 伪头部的结构
type pseudoHeader struct {
	SourceAddress      [4]byte
	DestinationAddress [4]byte
	Zero               byte
	Protocol           byte
	TCPLength          uint16
}

// 创建伪头部数据
func makePseudoHeader(srcIP, dstIP string, tcpLen uint16) []byte {
	src := net.ParseIP(srcIP).To4()
	dst := net.ParseIP(dstIP).To4()
	pHeader := pseudoHeader{
		SourceAddress:      [4]byte{src[0], src[1], src[2], src[3]},
		DestinationAddress: [4]byte{dst[0], dst[1], dst[2], dst[3]},
		Zero:               0,
		Protocol:           6, // TCP 的协议号是 6
		TCPLength:          tcpLen,
	}

	buf := make([]byte, 12)
	binary.BigEndian.PutUint32(buf[0:], binary.BigEndian.Uint32(pHeader.SourceAddress[:]))
	binary.BigEndian.PutUint32(buf[4:], binary.BigEndian.Uint32(pHeader.DestinationAddress[:]))
	buf[8] = pHeader.Zero
	buf[9] = pHeader.Protocol
	binary.BigEndian.PutUint16(buf[10:], pHeader.TCPLength)

	return buf
}

func Checksum(src, dst string, data []byte) uint16 {
	tcpLength := uint16(len(data))
	pseudoHeader := makePseudoHeader(src, dst, tcpLength)
	fullData := append(pseudoHeader, data...)
	check := checksum(fullData)
	return check
}
