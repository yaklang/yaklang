package yakit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type initializingCallback struct {
	Note string
	Fn   func() error
}

var (
	__initializingDatabase []*initializingCallback
	__mutexForInit         = new(sync.Mutex)
)

type DbExecFunc func(db *gorm.DB) error

var DBSaveAsyncChannel = make(chan DbExecFunc, 40960)

func init() {
	throttle := utils.NewThrottle(2)
	go func() {
		var count uint64 = 0
		for {
			select {
			case f := <-DBSaveAsyncChannel:
				start := time.Now()
				err := f(consts.GetGormProjectDatabase())
				elapsed := time.Since(start)
				if elapsed > 2*time.Second {
					log.Warnf("SQL execution took too long: %v, function: %+v", elapsed, f)
				}
				count++
				if count%1000 == 0 {
					throttle(func() {
						//log.Infof("Throttle sql exec count: %d", count)
					})
				}
				if err != nil {
					log.Errorf("Throttle sql exec failed: %s", err)
				}
			}
		}
	}()
}

func RegisterPostInitDatabaseFunction(f func() error, notes ...string) {
	__mutexForInit.Lock()
	defer __mutexForInit.Unlock()
	__initializingDatabase = append(__initializingDatabase, &initializingCallback{
		Note: strings.Join(notes, ";"),
		Fn: func() error {
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("PostInitDatabaseFunction panic: %v\n%s", r, spew.Sdump(r))
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()
			return f()
		},
	})
}

const md5Placeholder = "f97f966eb7f8ba8fdb63e4d29109c058" // md5("CallPostInitDatabase")

func CallPostInitDatabase() error {
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			// if the context timed out, echo a warning for frontend
			msg := "CallPostInitDatabase is taking too long, please wait..."

			// log for frontend
			m := make(map[string]any)
			m["warning"] = msg
			msgBytes, _ := json.Marshal(m)
			log.Warnf("<json-%s>%s<json-%s>\n",
				md5Placeholder, string(msgBytes), md5Placeholder,
			)

			// log for console
			log.Warn(msg)
		}
	}()

	for idx, f := range __initializingDatabase {
		if f == nil || f.Fn == nil {
			continue
		}
		currentFuncStart := time.Now()
		err := f.Fn()
		elapsed := time.Since(currentFuncStart)
		if elapsed > 1*time.Second {
			node := f.Note
			if node == "" {
				node = fmt.Sprint(idx)
			}
			log.Warnf("call post function[%v] took too long: %v", node, elapsed)
		}
		if err != nil {
			return err
		}
	}
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		log.Warnf("call post functions took too long: %v", elapsed)
	}
	return nil
}

func InitialDatabase() {
	consts.GetGormProfileDatabase()
	consts.GetGormProjectDatabase()
	err := CallPostInitDatabase()
	if err != nil {
		log.Errorf(`yakit.CallPostInitDatabase failed: %s`, err)
	}
}
