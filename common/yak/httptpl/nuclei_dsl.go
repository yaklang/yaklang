package httptpl

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"github.com/asaskevich/govalidator"
	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/go-version"
	"math"
	"math/rand"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"yaklang/common/go-funk"
	"yaklang/common/log"
	"yaklang/common/mutate"
	"yaklang/common/utils"
	"yaklang/common/utils/jodatime"
	"yaklang/common/utils/lowhttp"
	"yaklang/common/yak/antlr4yak"
	"yaklang/common/yak/antlr4yak/yakvm"
	"yaklang/common/yak/yaklang"
	"yaklang/common/yak/yaklang/lib/builtin"
	"yaklang/common/yak/yaklib/codec"
)

// ns_sort: sort a list of numbers or strings
func nc_sort(origin ...interface{}) (ret []interface{}) {
	if len(origin) == 0 {
		return nil
	}

	defer func() {
		if err := recover(); err != nil {
			ret = origin
			log.Warnf("sort error: %v", err)
			return
		}
	}()
	sample := origin[0]
	switch sample.(type) {
	case int:
		sort.SliceStable(origin, func(i, j int) bool {
			return origin[i].(float64) > origin[j].(float64)
		})
	case float64:
		sort.SliceStable(origin, func(i, j int) bool {
			return origin[i].(float64) > origin[j].(float64)
		})
		return origin
	case string:
		sort.SliceStable(origin, func(i, j int) bool {
			return origin[i].(string) > origin[j].(string)
		})
	}
	return origin
}

func toString(i interface{}) string {
	return utils.InterfaceToString(i)
}

var nucleiDSLFunctions = map[string]interface{}{
	"dump":     spew.Dump,
	"len":      builtin.Len,
	"to_upper": strings.ToUpper,
	"toupper":  strings.ToUpper,
	"to_lower": strings.ToLower,
	"tolower":  strings.ToLower,
	"sort":     nc_sort,
	"uniq": func(args ...interface{}) interface{} {
		// Unique String
		argCount := len(args)
		if argCount == 0 {
			return args
		} else if argCount == 1 {
			builder := &strings.Builder{}
			visited := make(map[rune]struct{})
			for _, i := range toString(args[0]) {
				if _, isRuneSeen := visited[i]; !isRuneSeen {
					builder.WriteRune(i)
					visited[i] = struct{}{}
				}
			}
			return builder.String()
		} else {
			result := make([]string, 0, argCount)
			visited := make(map[string]struct{})
			for _, i := range args[0:] {
				if _, isStringSeen := visited[toString(i)]; !isStringSeen {
					result = append(result, toString(i))
					visited[toString(i)] = struct{}{}
				}
			}
			return result
		}
	},
	"repeat": func(i interface{}, count int) string {
		return strings.Repeat(toString(i), count)
	},
	"replace": func(i, old, new interface{}) string {
		return strings.ReplaceAll(toString(i), toString(old), toString(new))
	},
	"replace_regex": func(i, old, new interface{}) string {
		compiled, err := regexp.Compile(toString(old))
		if err != nil {
			log.Error(err)
			return toString(i)
		}
		return compiled.ReplaceAllString(toString(i), toString(new))
	},
	"trim": func(i interface{}, i2 interface{}) string {
		return strings.Trim(toString(i), toString(i2))
	},
	"trim_left": func(i interface{}, i2 interface{}) string {
		return strings.TrimLeft(toString(i), toString(i2))
	},
	"trim_right": func(i interface{}, i2 interface{}) string {
		return strings.TrimRight(toString(i), toString(i2))
	},
	"trim_space": func(i interface{}) string {
		return strings.TrimSpace(toString(i))
	},
	"reverse": func(i interface{}) string {
		return utils.StringReverse(toString(i))
	},
	"base64": func(i interface{}) string {
		return codec.EncodeBase64(i)
	},
	"gzip": func(i interface{}) string {
		ret, err := utils.GzipCompress(toString(i))
		if err != nil {
			log.Error(err)
			return ""
		}
		return string(ret)
	},
	"gzip_decode": func(i interface{}) string {
		raw, err := utils.GzipDeCompress([]byte(toString(i)))
		if err != nil {
			log.Error(err)
			return ""
		}
		return string(raw)
	},
	"zlib": func(i interface{}) string {
		raw, err := utils.ZlibCompress([]byte(toString(i)))
		if err != nil {
			log.Error(err)
			return ""
		}
		return string(raw)
	},
	"zlib_decode": func(i interface{}) string {
		raw, err := utils.ZlibDeCompress([]byte(toString(i)))
		if err != nil {
			log.Error(err)
			return ""
		}
		return string(raw)
	},
	"date_time": func(fmtStr string, i interface{}) string {
		switch ret := i.(type) {
		case int64:
			return jodatime.Format(fmtStr, time.Unix(ret, 0))
		case time.Time:
			return jodatime.Format(fmtStr, ret)
		}
		log.Errorf("`date_time` cannot handle: %v", spew.Sdump(i))
		return ""
	},
	"base64_py": func(i interface{}) string {
		return string(bytes.Join(funk.Chunk([]byte(codec.EncodeBase64(i)), 76).([][]byte), []byte("\n")))
	},
	"base64_decode": func(i interface{}) string {
		raw, err := codec.DecodeBase64(toString(i))
		if err != nil {
			log.Error(err)
			return ""
		}
		return string(raw)
	},
	"url_encode": func(i interface{}) string {
		return codec.QueryEscape(toString(i))
	},
	"url_decode": func(i interface{}) string {
		raw, err := codec.QueryUnescape(toString(i))
		if err != nil {
			log.Error(err)
			return toString(i)
		}
		return raw
	},
	"hex_encode": codec.EncodeToHex,
	"hex_decode": func(i interface{}) string {
		raw, err := codec.DecodeHex(toString(i))
		if err != nil {
			log.Error(err)
			return ""
		}
		return string(raw)
	},
	"hmac": func(alg string, data, secret string) string {
		switch strings.ToLower(alg) {
		case "sha1", "sha-1":
			return string(codec.HmacSha1(secret, data))
		case "sha256", "sha-256":
			return string(codec.HmacSha256(secret, data))
		case "sha512", "sha-512":
			return string(codec.HmacSha512(secret, data))
		case "md5":
			return string(codec.HmacMD5(secret, data))
		case "sm3":
			return string(codec.HmacSM3(secret, data))
		default:
			log.Error("no-supported alg: " + alg)
			return ""
		}
	},
	"html_escape":   codec.EncodeHtmlEntity,
	"html_unescape": codec.UnescapeHtmlString,
	"md5":           codec.Md5,
	"sha512":        codec.Sha512,
	"sha256":        codec.Sha256,
	"sha1":          codec.Sha1,
	"sm3":           codec.SM3,
	"mmh3":          codec.MMH3Hash32,
	"concat": func(i ...interface{}) string {
		var buf bytes.Buffer
		for _, ret := range i {
			buf.WriteString(fmt.Sprint(ret))
		}
		return buf.String()
	},
	"contains": func(i any, elems ...interface{}) bool {
		if len(elems) <= 0 {
			return false
		}

		_, ok := i.(string)
		_, ok2 := i.([]byte)
		_, ok3 := i.([]rune)
		if ok || ok2 || ok3 {
			for _, elem := range elems {
				if !strings.Contains(toString(i), toString(elem)) {
					return false
				}
			}
			return true
		}

		for _, elem := range elems {
			if !funk.Contains(i, fmt.Sprint(elem)) {
				return false
			}
		}
		return true
	},
	"contains_any": func(i any, elems ...interface{}) bool {
		for _, elem := range elems {
			if funk.Contains(i, fmt.Sprint(elem)) {
				return true
			}
		}
		return false
	},
	"starts_with": func(i string, pres ...string) bool {
		for _, prefix := range pres {
			if strings.HasPrefix(i, prefix) {
				return true
			}
		}
		return false
	},
	"line_starts_with": func(i string, pres ...string) bool {
		for _, line := range utils.ParseStringToLines(i) {
			for _, pre := range pres {
				if strings.HasPrefix(line, pre) {
					return true
				}
			}
		}
		return false
	},
	"ends_with": func(i string, sufs ...string) bool {
		for _, prefix := range sufs {
			if strings.HasSuffix(i, prefix) {
				return true
			}
		}
		return false
	},
	"line_ends_with": func(i string, items ...string) bool {
		for _, line := range utils.ParseStringToLines(i) {
			for _, pre := range items {
				if strings.HasSuffix(line, pre) {
					return true
				}
			}
		}
		return false
	},
	"split": func(input string, args ...interface{}) []string {
		switch len(args) {
		case 0:
			return utils.ParseStringToLines(input)
		case 1:
			switch ret := args[0].(type) {
			case int:
				var res []string
				for _, l := range funk.Chunk([]byte(input), ret).([][]byte) {
					res = append(res, string(l))
				}
				return res
			default:
				return strings.SplitN(input, toString(ret), -1)
			}
		case 2:
			n := -1
			if ret, err := strconv.Atoi(toString(args[1])); err != nil {
				n = -1
			} else {
				n = ret
			}
			return strings.SplitN(input, toString(args[0]), n)
		default:
			return utils.ParseStringToLines(input)
		}
	},
	"join": func(sep interface{}, items ...interface{}) string {
		sepStr := toString(sep)
		return strings.Join(utils.InterfaceToStringSlice(items), sepStr)
	},
	"regex": func(r string, i interface{}) bool {
		result, _ := regexp.MatchString(r, toString(i))
		return result
	},
	"regex_all": func(r string, i ...interface{}) bool {
		if len(i) == 0 {
			return false
		}
		for _, item := range i {
			result, _ := regexp.MatchString(r, toString(item))
			if !result {
				return false
			}
		}
		return true
	},
	"regex_any": func(r string, i ...interface{}) bool {
		for _, item := range i {
			result, _ := regexp.MatchString(r, toString(item))
			if result {
				return true
			}
		}
		return false
	},
	"equals_any": func(origin string, req ...interface{}) bool {
		if len(req) == 0 {
			return false
		}
		return utils.StringArrayContains(utils.InterfaceToStringSlice(req), origin)
	},
	"remove_bad_chars": func(i string, cutset string) string {
		for _, c := range cutset {
			i = strings.ReplaceAll(i, string(c), "")
		}
		return i
	},
	"rand_char": func(n int, i ...interface{}) string {
		if len(i) == 0 {
			return utils.RandStringBytes(n)
		}
		return utils.RandSample(n, utils.InterfaceToStringSlice(i)...)
	},
	"rand_base": func(i int, charSets ...string) string {
		charsetStr := strings.Join(charSets, "")
		if charsetStr != "" {
			return utils.RandSample(i, charsetStr)
		}
		return utils.RandStringBytes(i)
	},
	"rand_text_alphanumeric": func(n int, bad ...interface{}) string {
		base := utils.LittleChar + utils.BigChar + utils.NumberChar
		if len(bad) > 0 {
			for _, i := range strings.Join(utils.InterfaceToStringSlice(bad), "") {
				base = strings.ReplaceAll(base, string([]rune{i}), "")
			}
		}
		if base == "" {
			base = utils.LittleChar + utils.BigChar + utils.NumberChar
		}
		return utils.RandSample(n, base)
	},
	"rand_text_alpha": func(n int, bad ...interface{}) string {
		base := utils.LittleChar + utils.BigChar
		if len(bad) > 0 {
			for _, i := range strings.Join(utils.InterfaceToStringSlice(bad), "") {
				base = strings.ReplaceAll(base, string([]rune{i}), "")
			}
		}
		if base == "" {
			base = utils.LittleChar + utils.BigChar
		}
		return utils.RandSample(n, base)
	},
	"rand_text_numeric": func(n int, bad ...interface{}) string {
		base := utils.NumberChar
		if len(bad) > 0 {
			for _, i := range strings.Join(utils.InterfaceToStringSlice(bad), "") {
				base = strings.ReplaceAll(base, string([]rune{i}), "")
			}
		}
		if base == "" {
			base = utils.NumberChar
		}
		return utils.RandSample(n, base)
	},
	"rand_int": func(args ...int) int {
		var min, max int
		switch len(args) {
		case 1:
			max = args[0]
		case 2:
			min = args[0]
			max = args[1]
			if max >= min {
				break
			}
			min, max = max, min
		default:
			min = 0
			max = math.MaxInt64
		}
		if max == 0 {
			max = math.MaxInt64
		}
		return min + rand.Intn(max-min)
	},
	"rand_ip": func(cidr ...string) string {
		sample := utils.ParseStringToHosts(strings.Join(cidr, ","))
		if len(sample) > 0 {
			return sample[rand.Intn(len(sample))]
		}
		results := mutate.MutateQuick(`{{ri(1,255)}}.{{ri(1,255)}}.{{ri(1,255)}}.{{ri(1,255)}}`)
		if len(results) > 0 {
			return results[0]
		}
		log.Error("fetch random ip failed")
		return "127.0.0.1"
	},
	"generate_java_gadget": func(gadget, cmd, encoding string) string {
		log.Info("nuclei generate java gadget failed")
		return ""
	},
	"unix_time": func(offset ...int64) int64 {
		var offsetInt int64 = 0
		if len(offset) > 0 {
			offsetInt = offset[0]
		}
		return time.Now().Add(time.Duration(offsetInt) * time.Second).Unix()
	},
	"to_unix_time": func(t string, layouts ...string) int64 {
		nr, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return int64(nr)
		}

		if len(layouts) > 0 {
			for _, layout := range layouts {
				timeIns, err := time.Parse(t, layout)
				if err != nil {
					continue
				}
				if timeIns.Unix() > 0 {
					return timeIns.Unix()
				}

				timeIns, err = jodatime.Parse(t, layout)
				if err != nil {
					continue
				}
				if timeIns.Unix() > 0 {
					return timeIns.Unix()
				}
			}
			return 0
		}

		for _, layout := range defaultDateTimeLayouts {
			timeIns, err := time.Parse(t, layout)
			if err != nil {
				continue
			}
			if timeIns.Unix() > 0 {
				return timeIns.Unix()
			}
		}
		return 0
	},
	"wait_for": func(i float64) {
		time.Sleep(utils.FloatSecondDuration(i))
	},
	"compare_versions": func(v1 string, opts ...string) bool {
		if len(opts) <= 0 {
			return false
		}
		firstVersion, err := version.NewVersion(v1)
		if err != nil {
			log.Errorf("compare versions failed, parse version str[%v]: %v", v1, err)
			return false
		}
		constraints, err := version.NewConstraint(strings.Join(opts, ","))
		if err != nil {
			log.Errorf("parse opts %v failed: %s", strings.Join(opts, ","), err)
			return false
		}
		return constraints.Check(firstVersion)
	},
	"print_debug": func(i ...interface{}) {
		spew.Dump(i)
	},
	"to_number": func(i ...interface{}) interface{} {
		raw := strings.Join(utils.InterfaceToStringSlice(i), "")
		if govalidator.IsInt(raw) {
			return atoi(raw)
		}

		if govalidator.IsFloat(raw) {
			return atof(raw)
		}

		return 0
	},
	"to_string": func(i ...interface{}) string {
		return strings.Join(utils.InterfaceToStringSlice(i), "")
	},
	"hex_to_dec": func(h string) int {
		raw, err := stringNumberToDecimal(h, "0x", 16)
		if err != nil {
			log.Error(err)
		}
		return int(raw)
	},
	"oct_to_dec": func(o string) int {
		raw, err := stringNumberToDecimal(o, "0o", 8)
		if err != nil {
			log.Error(err)
		}
		return int(raw)
	},
	"bin_to_dec": func(b string) int {
		raw, err := stringNumberToDecimal(b, "0b", 2)
		if err != nil {
			log.Error(err)
		}
		return int(raw)
	},
	"substr": func(str string, start int, endArgs ...int) string {
		if len(endArgs) > 0 {
			if endArgs[0] > len(str) {
				endArgs[0] = len(str)
			}
			if start > endArgs[0] {
				start = endArgs[0]
			}
			return str[start:endArgs[0]]
		}
		if start > len(str) {
			start = len(str)
		}
		return str[start:]
	},
	"aes_cbc": func(data, key, iv string) string {
		bPlainText := codec.PKCS5Padding([]byte(data), aes.BlockSize)
		block, _ := aes.NewCipher([]byte(key))
		ciphertext := make([]byte, len(bPlainText))
		mode := cipher.NewCBCEncrypter(block, []byte(iv))
		mode.CryptBlocks(ciphertext, bPlainText)
		return string(ciphertext)
	},
	"aes_gcm": func(data, key, iv string) string {
		log.Infof("error: aes_gcm not implemented")
		return ""
	},
}

func stringNumberToDecimal(input string, prefix string, base int) (int64, error) {
	if strings.HasPrefix(input, prefix) {
		base = 0
	}
	if number, err := strconv.ParseInt(input, base, 64); err == nil {
		return number, err
	}
	return 0, fmt.Errorf("invalid number: %s", input)
}

func atoi(i string) int {
	raw, _ := strconv.Atoi(i)
	return raw
}

func atof(i string) float64 {
	raw, _ := strconv.ParseFloat(i, 64)
	return raw
}

var defaultDateTimeLayouts = []string{
	time.RFC3339,
	"2006-01-02 15:04:05 Z07:00",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04 Z07:00",
	"2006-01-02 15:04",
	"2006-01-02 Z07:00",
	"2006-01-02",
}

func init() {
	nucleiDSLFunctions["contains_all"] = nucleiDSLFunctions["contains"]
}

func GetNucleiDSLFunctions() map[string]interface{} {
	libs := make(map[string]interface{})
	for k, v := range nucleiDSLFunctions {
		libs[k] = v
	}
	return nucleiDSLFunctions
}

type NucleiDSL struct {
	Functions map[string]interface{}
}

func NewNucleiDSLYakSandbox() *NucleiDSL {
	dsl := &NucleiDSL{Functions: make(map[string]interface{})}
	return dsl
}

func LoadVarFromRawResponse(rsp []byte, duration float64, sufs ...string) map[string]interface{} {
	rs := make(map[string]interface{})
	var (
		contentLength = 0
		headers       = make(map[string]string)
		raw           = []byte(string(rsp))
		statusCode    = 0
	)

	headerRaw, body := lowhttp.SplitHTTPHeadersAndBodyFromPacketEx2(rsp, nil, func(proto string, code int, codeMsg string) error {
		statusCode = code
		return nil
	}, func(line string) {
		k, v := lowhttp.SplitHTTPHeader(line)
		exportedKey := strings.ReplaceAll(strings.ToLower(k), "-", "_")
		headers[exportedKey] = v
	})
	contentLength = len(body)

	for k, v := range headers {
		rs[k] = v
	}
	rs["all_headers"] = headerRaw
	rs["status_code"] = statusCode
	rs["content_length"] = contentLength
	rs["body"] = string(body)
	rs["raw"] = raw
	rs["duration"] = duration

	if len(sufs) > 0 {
		vars := utils.CopyMapInterface(rs)
		for _, i := range sufs {
			for k, v := range rs {
				vars[k+utils.InterfaceToString(i)] = v
			}
		}
		return vars
	}
	return rs
}

func (d *NucleiDSL) _execute(expr string, items ...map[string]interface{}) (*antlr4yak.Engine, map[string]interface{}, error) {
	box := yaklang.NewSandbox(GetNucleiDSLFunctions())
	merged := make(map[string]interface{})
	for _, v := range items {
		if v == nil {
			continue
		}
		for k, v := range v {
			merged[k] = v
		}
	}
	return box, merged, nil
}

func (d *NucleiDSL) Execute(expr string, items ...map[string]interface{}) (interface{}, error) {
	box, merged, err := d._execute(expr, items...)
	if err != nil {
		return nil, err
	}
	return box.ExecuteAsExpression(expr, merged)
}

func (d *NucleiDSL) ExecuteAsBool(expr string, items ...map[string]interface{}) (bool, error) {
	box, merged, err := d._execute(expr, items...)
	if err != nil {
		return false, err
	}
	return box.ExecuteAsBooleanExpression(expr, merged)
}

func (d *NucleiDSL) GetUndefinedVarNames(expr string, extra map[string]interface{}) []string {
	vars := []string{}
	funcs := GetNucleiDSLFunctions()
	box := yaklang.NewSandbox(funcs)
	codes, err := box.Compile(expr)
	for _, code := range codes {
		switch code.Opcode {
		case yakvm.OpPushId:
			varName := code.Op1.String()
			_, ok := funcs[varName]
			if ok {
				continue
			}
			if extra != nil {
				_, ok = extra[varName]
				if ok {
					continue
				}
			}

			vars = append(vars, varName)
		}
	}
	if err != nil {
		log.Warnf("compile vars (%v) failed: %s", expr, err)
	}
	return vars
}

func IsExprReady(expr string, m map[string]interface{}) (bool, []string) {
	empty := NewNucleiDSLYakSandbox().GetUndefinedVarNames(expr, m)
	return len(empty) == 0, empty
}
