package yaklib

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklang/spec"
	"github.com/yaklang/yaklang/common/yakdocument"
	"io"
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
)

func _sfmt(f string, items ...interface{}) string {
	return fmt.Sprintf(f, items...)
}

func _assert(b bool, reason ...interface{}) {
	if !b {
		panic(spew.Sdump(reason))
	}
}

func _failed(msg ...interface{}) {
	if msg == nil {
		panic("exit")
	}

	var msgs []string
	for _, i := range msg {
		if err, ok := i.(error); ok {
			msgs = append(msgs, err.Error())
		} else if s, ok := i.(string); ok {
			msgs = append(msgs, s)
		} else {
			msgs = append(msgs, spew.Sdump(i))
		}
	}
	panic(strings.Join(msgs, "\n"))
}
func yakitOutputHelper(i interface{}) {
	if yakitClientInstance != nil {
		level, data := MarshalYakitOutput(i)
		yakitClientInstance.YakitLog(level, data)
	}
}
func _diewith(err interface{}) {
	if err == nil {
		return
	}
	yakitOutputHelper(fmt.Sprintf("YakVM Code DIE With Err: %v", spew.Sdump(err)))
	_failed(err)
}

func _logDiscard() {
	log.SetOutput(io.Discard)
}

func _logRecover() {
	log.SetOutput(os.Stdout)
}
func dummyN(item ...interface{}) {
	spew.Dump(item)
}

var GlobalExport = map[string]interface{}{
	"_createOnLogger":        createLogger,
	"_createOnLoggerConsole": createConsoleLogger,
	"_createOnFailed":        createFailed,
	"_createOnOutput":        createOnOutput,
	"_createOnFinished":      createOnFinished,
	"_createOnAlert":         createOnAlert,

	"loglevel":     setLogLevel,
	"logquiet":     _logDiscard,
	"logdiscard":   _logDiscard,
	"logrecover":   _logRecover,
	"yakit_output": dummyN,
	"yakit_save":   dummyN,
	"yakit_status": dummyN,

	// die error
	"fail": _failed,
	"die":  _diewith,
	"uuid": func() string {
		return uuid.New().String()
	},
	"timestamp": func() int64 {
		return time.Now().Unix()
	},
	"nanotimestamp": func() int64 {
		return time.Now().UnixNano()
	},
	"datetime": func() string {
		return time.Now().Format("2006-01-02 15:04:05")
	},
	"date": func() string {
		return time.Now().Format("2006-01-02")
	},
	"now": time.Now,

	"timestampToDatetime": func(tValue int64) string {
		tm := time.Unix(tValue, 0)
		return tm.Format("2006-01-02 15:04:05")
	},
	"datetimeToTimestamp": func(str string) (int64, error) {
		t, err := time.Parse("2006-01-02 15:04:05", str)
		if err != nil {
			return 0, err
		}
		return t.Unix(), nil
	},
	"timestampToTime": func(i int64) time.Time {
		return time.Unix(i, 0)
	},
	"dump": func(i ...interface{}) {
		spew.Dump(i...)
	},
	"sdump": func(i ...interface{}) string {
		return spew.Sdump(i...)
	},
	"randn": func(min, max int) int {
		if min > max {
			panic(_sfmt("randn failed; min: %v max: %v", min, max))
		}
		return min + rand.Intn(max-min)
	},
	"randstr": func(length int) string {
		return utils.RandStringBytes(length)
	},
	"sleep": sleep,
	"wait": func(i interface{}) {
		switch ret := i.(type) {
		case context.Context:
			select {
			case <-ret.Done():
			}
		case string:
			sleep(parseFloat(ret))
		case float64:
			sleep(ret)
		case float32:
			sleep(float64(ret))
		case int:
			sleep(float64(ret))
		default:
			panic(fmt.Sprintf("cannot wait %v", spew.Sdump(ret)))
		}

	},
	"assert":     _assert,
	"assertTrue": _assert,
	"isEmpty": func(i interface{}) bool {
		if i == nil || i == spec.Undefined {
			return true
		}
		return false
	},
	"assertEmpty": func(i interface{}) {
		if i == nil || i == spec.Undefined {
			return
		}
		panic(_sfmt("expect nil but got %v", spew.Sdump(i)))
	},
	"assertf": func(b bool, f string, items ...interface{}) {
		if !b {
			panic(_sfmt(f, items))
		}
	},

	"parseInt":     parseInt,
	"parseStr":     parseString,
	"parseString":  parseString,
	"parseBool":    parseBool,
	"parseBoolean": parseBool,
	"parseFloat":   parseFloat,
	"atoi":         strconv.Atoi,
	"parseTime":    time.Parse,
	"input":        _input,

	// 每一秒执行一次
	"tick1s": tick1s,

	"desc":    _desc,
	"descStr": _descToString,
	"chr": func(i interface{}) string {
		return string([]byte{byte(parseInt(utils.InterfaceToString(i)))})
	},
	"ord": func(i interface{}) int {
		switch ret := i.(type) {
		case byte:
			return int(ret)
		default:
			strRaw := utils.InterfaceToString(i)
			if strRaw == "" {
				return -1
			}

			if r := []rune(strRaw); r != nil {
				return int(r[0])
			}

			return int(strRaw[0])
		}
	},
	"typeof": func(i interface{}) reflect.Type {
		return reflect.TypeOf(i)
	},
}

func _desc(i interface{}) {
	info, err := yakdocument.Dir(i)
	if err != nil {
		log.Error(err)
		return
	}
	info.Show()
}

func _descToString(i interface{}) string {
	info, err := yakdocument.Dir(i)
	if err != nil {
		log.Error(err)
		return ""
	}
	return info.String()
}

func tick1s(f func() bool) {
	t := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-t.C:
			if !f() {
				return
			}
		}
	}
}

func sleep(i float64) {
	time.Sleep(utils.FloatSecondDuration(i))
}
