package yakit

import (
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	__initializingDatabase []func() error
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
					log.Warnf("SQL execution took too long: %v", elapsed)
				}
				count++
				if count%1000 == 0 {
					throttle(func() {
						log.Infof("Throttle sql exec count: %d", count)
					})
				}
				if err != nil {
					log.Errorf("Throttle sql exec failed: %s", err)
				}
			}
		}
	}()
}

func RegisterPostInitDatabaseFunction(f func() error) {
	__mutexForInit.Lock()
	defer __mutexForInit.Unlock()
	__initializingDatabase = append(__initializingDatabase, f)
}

func CallPostInitDatabase() error {
	for _, f := range __initializingDatabase {
		err := f()
		if err != nil {
			return err
		}
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
