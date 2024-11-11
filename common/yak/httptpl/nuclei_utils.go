package httptpl

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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

func toBytes(i interface{}) []byte {
	return utils.InterfaceToBytes(i)
}

func ExtractResultToString(i interface{}) string {
	switch v := i.(type) {
	case string:
		return utils.EscapeInvalidUTF8Byte([]byte(v))
	case []byte:
		return utils.EscapeInvalidUTF8Byte(v)
	default:
		return strings.Join(lo.Map(utils.InterfaceToStringSlice(i), func(item string, index int) string {
			return utils.EscapeInvalidUTF8Byte([]byte(item))
		}), ",")
	}
}

func parseTimeOrNow(arguments []interface{}) (time.Time, error) {
	var currentTime time.Time
	if len(arguments) == 2 {
		switch inputUnixTime := arguments[1].(type) {
		case time.Time:
			currentTime = inputUnixTime
		case string:
			unixTime, err := strconv.ParseInt(inputUnixTime, 10, 64)
			if err != nil {
				return time.Time{}, errors.New("invalid argument type")
			}
			currentTime = time.Unix(unixTime, 0)
		case int64, float64:
			currentTime = time.Unix(int64(inputUnixTime.(float64)), 0)
		default:
			return time.Time{}, errors.New("invalid argument type")
		}
	} else {
		currentTime = time.Now()
	}
	return currentTime, nil
}

func WhatsMyIP() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://checkip.amazonaws.com/", nil)
	if err != nil {
		return "", nil
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error fetching ip: %s", resp.Status)
	}

	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.Trim(string(ip), "\n\r\t "), nil
}

func GetPublicIP() string {
	PublicIPGetOnce.Do(func() {
		publicIP, _ = WhatsMyIP()
	})
	return publicIP
}

func _index(arg any, index int64) interface{} {
	// If the first argument is a slice, we index into it
	switch v := arg.(type) {
	case []string:
		l := int64(len(v))
		if index < 0 || index >= l {
			log.Errorf("index out of range for %v: %d", v, index)
			return nil
		}
		return v[index]
	default:
		// Otherwise, we index into the string
		str := toString(v)
		l := int64(len(str))
		if index < 0 || index >= l {
			log.Errorf("index out of range for %v: %d", v, index)
			return nil
		}
		return string(str[index])
	}
}

func gadgetEncodingHelper(returnData []byte, encoding string) string {
	switch encoding {
	case "raw":
		return string(returnData)
	case "hex":
		return hex.EncodeToString(returnData)
	case "gzip":
		buffer := &bytes.Buffer{}
		writer := gzip.NewWriter(buffer)
		if _, err := writer.Write(returnData); err != nil {
			return ""
		}
		_ = writer.Close()
		return buffer.String()
	case "gzip-base64":
		buffer := &bytes.Buffer{}
		writer := gzip.NewWriter(buffer)
		if _, err := writer.Write(returnData); err != nil {
			return ""
		}
		_ = writer.Close()
		return urlsafeBase64Encode(buffer.Bytes())
	case "base64-raw":
		return base64.StdEncoding.EncodeToString(returnData)
	default:
		return urlsafeBase64Encode(returnData)
	}
}

func urlsafeBase64Encode(data []byte) string {
	return strings.ReplaceAll(base64.StdEncoding.EncodeToString(data), "+", "%2B")
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
