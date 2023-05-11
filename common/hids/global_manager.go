package hids

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/healthinfo"
	"sync"
	"time"
)

var monitorDuration = 5 * time.Second
var setGlobalHealthManagerMutex sync.Mutex
var _globalHealthManager *healthinfo.Manager

func resetGlobalHealthManager() {
	setGlobalHealthManagerMutex.Lock()
	defer setGlobalHealthManagerMutex.Unlock()

	if _globalHealthManager != nil {
		_globalHealthManager.Cancel()
	}
	_globalHealthManager = nil
}

func setGlobalHealthManager(i *healthinfo.Manager) {
	resetGlobalHealthManager()

	setGlobalHealthManagerMutex.Lock()
	_globalHealthManager = i
	setGlobalHealthManagerMutex.Unlock()
}

func GetGlobalHealthManager() *healthinfo.Manager {
	if _globalHealthManager == nil {
		m, err := healthinfo.NewHealthInfoManager(monitorDuration, 30*time.Minute)
		if err != nil {
			log.Warnf("cannot create health-info-manager, reason: %s", err)
			return nil
		}
		setGlobalHealthManager(m)
		return m
	}
	return _globalHealthManager
}

func SetMonitorIntervalFloat(i float64) {
	if i < 1 {
		log.Warnf("invalid monitor-interval: %fs, at least 1s", i)
		return
	}
	monitorDuration = utils.FloatSecondDuration(i)

	if _globalHealthManager != nil {
		log.Info("monitor duration(interval) has been modified, reset health manager...")
		resetGlobalHealthManager()
		GetGlobalHealthManager()
	}
}

func InitHealthManager() {
	GetGlobalHealthManager()
}

func ShowMonitorInterval() {
	fmt.Println(monitorDuration.String())
}
