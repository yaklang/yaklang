package yakit

import (
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

var (
	__initializingDatabase []func() error
	__mutexForInit         = new(sync.Mutex)
)

type DbExecFunc func(db *gorm.DB) error

var DBSaveAsyncChannel = make(chan DbExecFunc, 1000)

func init() {
	go func() {
		for {
			select {
			case f := <-DBSaveAsyncChannel:
				err := f(consts.GetGormProjectDatabase())
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
	__mutexForInit.Lock()
	defer __mutexForInit.Unlock()
	go func() {
		for _, f := range __initializingDatabase {
			err := f()
			if err != nil {
				log.Warnf("CallPostInitDatabase failed: %s", err)
				return
			}
		}
	}()
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
