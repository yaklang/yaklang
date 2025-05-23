package codec

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"reflect"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

// Credits: https://gist.github.com/zhangbaohe/c691e1da5bbdc7f41ca5

// Decodegbk converts GBK to UTF-8
func Decodegbk(s []byte) ([]byte, error) {
	I := bytes.NewReader(s)
	O := transform.NewReader(I, simplifiedchinese.GBK.NewDecoder())
	d, e := ioutil.ReadAll(O)
	if e != nil {
		return nil, e
	}
	return d, nil
}

// Decodebig5 converts BIG5 to UTF-8
func Decodebig5(s []byte) ([]byte, error) {
	I := bytes.NewReader(s)
	O := transform.NewReader(I, traditionalchinese.Big5.NewDecoder())
	d, e := ioutil.ReadAll(O)
	if e != nil {
		return nil, e
	}
	return d, nil
}

// Encodebig5 converts UTF-8 to BIG5
func Encodebig5(s []byte) ([]byte, error) {
	I := bytes.NewReader(s)
	O := transform.NewReader(I, traditionalchinese.Big5.NewEncoder())
	d, e := ioutil.ReadAll(O)
	if e != nil {
		return nil, e
	}
	return d, nil
}

func IsGBK(data []byte) bool {
	length := len(data)
	var i int = 0
	for i < length {
		if data[i] <= 0x7f {
			//编码0~127,只有一个字节的编码，兼容ASCII码
			i++
			continue
		} else {
			//非双字节编码 最后只剩一位
			if i+1 == length {
				return false
			}
			//大于127的使用双字节编码，落在gbk编码范围内的字符
			if data[i] >= 0x81 &&
				data[i] <= 0xfe &&
				data[i+1] >= 0x40 &&
				data[i+1] <= 0xfe &&
				data[i+1] != 0xf7 {
				i += 2
				continue
			} else {
				return false
			}
		}
	}
	return true
}

// UTF-8编码格式的判断
func preNUm(data byte) int {
	var mask byte = 0x80
	var num int = 0
	//8bit中首个0bit前有多少个1bits
	for i := 0; i < 8; i++ {
		if (data & mask) == mask {
			num++
			mask = mask >> 1
		} else {
			break
		}
	}
	return num
}

// UTF8AndControlEscapeForEditorView will remove some unfriendly chars for editor
func UTF8AndControlEscapeForEditorView(i any) string {
	var res = bytes.NewBuffer(nil)
	var raw = AnyToBytes(i)
	idx := 0
	for idx <= len(raw) {
		runeWord, n := utf8.DecodeRune(raw[idx:])
		if n == 0 {
			break
		}

		lastIdx := idx
		idx += n
		if runeWord == utf8.RuneError {
			word := raw[lastIdx]
			res.WriteString(`\x`)
			res.WriteString(fmt.Sprintf("%02x", word))
			continue
		}

		// break line
		if n == 1 {
			switch runeWord {
			case '\x00', // NULL
				'\x01', // Start of Heading
				'\x02', // Start of Text
				'\x03', // End of Text
				'\x04', // End of Transmission
				'\x05', // Enquiry
				'\x06', // Acknowledge
				'\x07', // Bell
				'\x08', // Backspace
				'\x0d', // Carriage Return
				'\x0e', // Shift Out
				'\x0f', // Shift In
				'\x10', // Data Link Escape
				'\x11', // Device Control One
				'\x12', // Device Control Two
				'\x13', // Device Control Three
				'\x14', // Device Control Four
				'\x15', // Negative Acknowledge
				'\x16', // Synchronous Idle
				'\x17', // End of Transmission Block
				'\x18', // Cancel
				'\x19', // End of Medium
				'\x1a', // Substitute
				'\x1b', // Escape
				'\x1c', // File Separator
				'\x1d', // Group Separator
				'\x1e', // Record Separator
				'\x1f': // Unit Separator
				continue
			}
			if runeWord > 0x7f {
				continue
			}
		} else if IsControl(runeWord) {
			continue
		}

		res.WriteRune(runeWord)
	}
	return res.String()
}

func UTF8SafeEscape(i any) string {
	var res = bytes.NewBuffer(nil)
	var raw = AnyToBytes(i)
	idx := 0
	for idx <= len(raw) {
		runeWord, n := utf8.DecodeRune(raw[idx:])
		if n == 0 {
			break
		}

		lastIdx := idx
		idx += n
		if runeWord == utf8.RuneError {
			word := raw[lastIdx]
			res.WriteString(`\x`)
			res.WriteString(fmt.Sprintf("%02x", word))
			continue
		}
		res.WriteRune(runeWord)
	}
	return res.String()
}

func StringUtf8SafeEscape(str string) string {
	if str == "" || utf8.ValidString(str) {
		return str
	}
	return UTF8SafeEscape(str)
}

func StringArrayUtf8SafeEscape(strArray []string) []string {
	for i := 0; i < len(strArray); i++ {
		strArray[i] = StringUtf8SafeEscape(strArray[i])
	}
	return strArray
}

// SanitizeUTF8 escapes invalid UTF-8 characters in a struct or slice.
func SanitizeUTF8(v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("v is nil")
	}
	return sanitizeRecursive(rv.Elem())
}

func sanitizeRecursive(val reflect.Value) error {
	switch val.Kind() {
	case reflect.Ptr:
		if !val.IsNil() {
			return sanitizeRecursive(val.Elem())
		}
	case reflect.Interface:
		if !val.IsNil() {
			return sanitizeRecursive(val.Elem())
		}
	case reflect.Struct:
		for i := 0; i < val.NumField(); i++ {
			if err := sanitizeRecursive(val.Field(i)); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			if err := sanitizeRecursive(val.Index(i)); err != nil {
				return err
			}
		}
	case reflect.Map:
		for _, key := range val.MapKeys() {
			originalKey := reflect.New(key.Type()).Elem()
			originalKey.Set(key)
			if err := sanitizeRecursive(originalKey); err != nil {
				return err
			}
			value := val.MapIndex(key)
			originalValue := reflect.New(value.Type()).Elem()
			originalValue.Set(value)
			if err := sanitizeRecursive(originalValue); err != nil {
				return err
			}
			val.SetMapIndex(key, reflect.Value{})
			val.SetMapIndex(originalKey, originalValue)
		}
	case reflect.String:
		if val.CanSet() {
			str := val.String()
			if !utf8.ValidString(str) {
				cleaned := UTF8SafeEscape(str)
				val.SetString(cleaned)
			}
		}
	default:

	}
	return nil
}

func IsUtf8(data []byte) bool {
	i := 0
	for i < len(data) {
		if (data[i] & 0x80) == 0x00 {
			// 0XXX_XXXX
			i++
			continue
		} else if num := preNUm(data[i]); num > 2 {
			// 110X_XXXX 10XX_XXXX
			// 1110_XXXX 10XX_XXXX 10XX_XXXX
			// 1111_0XXX 10XX_XXXX 10XX_XXXX 10XX_XXXX
			// 1111_10XX 10XX_XXXX 10XX_XXXX 10XX_XXXX 10XX_XXXX
			// 1111_110X 10XX_XXXX 10XX_XXXX 10XX_XXXX 10XX_XXXX 10XX_XXXX
			// preNUm() 返回首个字节的8个bits中首个0bit前面1bit的个数，该数量也是该字符所使用的字节数
			i++
			for j := 0; j < num-1; j++ {
				//判断后面的 num - 1 个字节是不是都是10开头
				if (data[i] & 0xc0) != 0x80 {
					return false
				}
				i++
			}
		} else {
			//其他情况说明不是utf-8
			return false
		}
	}
	return true
}

var controlChars = []rune{
	0x061C,
	0x200E,
	0x200F,
	0x202A,
	0x202B,
	0x202C,
	0x202D,
	0x202E,
	0x2066,
	0x2067,
	0x2068,
	0x2069,
	0xFFF9,
	0xFFFA,
	0xFFFB,
}

func IsControl(r rune) bool {
	return unicode.IsControl(r) || unicode.In(r, unicode.Bidi_Control) || unicode.In(r, unicode.Join_Control)
}
