package yakit

import (
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"sync"
	"time"
)

type AliveHostParamsOpt func(r *AliveHost)

func NewAliveHost(u string, opts ...AliveHostParamsOpt) (*AliveHost, error) {
	r := _createAliveHost(u, opts...)
	return r, _saveAliveHost(r)
}

var beforeAliveHostSave []func(*AliveHost)
var beforeAliveHostSaveMutex = new(sync.Mutex)

func _saveAliveHost(r *AliveHost) error {

	beforeAliveHostSaveMutex.Lock()
	defer beforeAliveHostSaveMutex.Unlock()
	for _, m := range beforeAliveHostSave {
		m(r)
	}

	db := consts.GetGormProjectDatabase()
	if db == nil {
		log.Error("empty database")
		return utils.Errorf("no database connection")
	}

	count := 0
	for {
		count++
		err := CreateOrUpdateAliveHost(db, r.Hash, r)
		if err != nil {
			if count < 20 {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			log.Errorf("save AliveHost failed: %s", err)
			return utils.Errorf("save AliveHost record failed: %s", err)
		}
		return nil
	}

}

func _createAliveHost(u string, opts ...AliveHostParamsOpt) *AliveHost {
	r := &AliveHost{
		Hash: uuid.NewV4().String(),
	}
	r.IP = u
	r.IPInteger, _ = utils.IPv4ToUint64(u)

	for _, opt := range opts {
		opt(r)
	}
	if r.RuntimeId == "" {
		r.RuntimeId = os.Getenv(consts.YAK_RUNTIME_ID)
	}

	return r
}
