package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"sync"
)

var __initializingDatabase []func() error
var __mutexForInit = new(sync.Mutex)

type DbExecFunc func(db *gorm.DB) error

var DBSaveAsyncChannel = make(chan DbExecFunc, 500)

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
	var err = CallPostInitDatabase()
	if err != nil {
		log.Errorf(`yakit.CallPostInitDatabase failed: %s`, err)
	}
}
