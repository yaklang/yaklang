package ssaprofile

import (
	"fmt"
	"slices"
	"sync"
	syncAtomic "sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

type Profile struct {
	Name      string
	TotalTime uint64
	Count     uint64
	Times     []uint64
}

func (profile *Profile) String() string {
	if profile == nil {
		return "----------- ProfileList [nil] --------------------"
	}

	ret := ""
	ret += fmt.Sprintf("----------- ProfileList [%s] --------------------", profile.Name)
	ret += fmt.Sprintf("\n-------- Profile %s\tCount %v\n", profile.Name, profile.Count)
	if profile.Count == 0 {
		return ret
	}

	ret += fmt.Sprintf("%s--all\tTime: %v\tCount: %v\tAvg: %v\n",
		profile.Name,
		time.Duration(profile.TotalTime),
		profile.Count,
		time.Duration(profile.TotalTime)/time.Duration(profile.Count),
	)
	for index, t := range profile.Times {
		ret += fmt.Sprintf("%s-%-4d\tTime: %v\tCount: %v\tAvg: %v\n",
			profile.Name, index+1,
			time.Duration(t),
			profile.Count,
			time.Duration(t)/time.Duration(profile.Count),
		)
	}
	return ret
}

var profileListMap = make(map[string]*Profile)
var lock = sync.RWMutex{}

func GetProfileListMap() map[string]*Profile {
	lock.RLock()
	defer lock.RUnlock()
	return profileListMap
}

func Refresh() {
	lock.Lock()
	defer lock.Unlock()
	profileListMap = make(map[string]*Profile)
}

func ProfileAdd(enable bool, name string, fs ...func()) {
	if name == "" {
		return
	}

	var p *Profile
	if enable {
		lock.RLock()
		p = profileListMap[name]
		lock.RUnlock()
		if p == nil {
			p = &Profile{
				Name:  name,
				Times: make([]uint64, 0, len(fs)),
			}
		}
	}

	var total uint64 = 0
	for index, f := range fs {
		start := time.Now()
		if f != nil {
			f()
		}
		since := uint64(time.Since(start))

		total += since
		if enable {
			if len(p.Times) <= index {
				p.Times = append(p.Times, 0)
			}
			syncAtomic.AddUint64(&p.Times[index], uint64(since))
		}
	}
	if enable {
		syncAtomic.AddUint64(&p.TotalTime, total)
		syncAtomic.AddUint64(&p.Count, 1)
		lock.Lock()
		profileListMap[name] = p
		lock.Unlock()
	}
}

func ShowCacheCost(pprof ...map[string]*Profile) {

	show := func(index int, prof map[string]*Profile) {
		profiles := make([]*Profile, 0, len(prof))
		for _, profile := range prof {
			profiles = append(profiles, profile)
		}

		slices.SortFunc(profiles, func(a, b *Profile) int {
			if a.TotalTime < b.TotalTime {
				return 1
			} else if a.TotalTime > b.TotalTime {
				return -1
			}
			return 0
		})
		log.Errorf("----------------------------------------[%d]--------------------------------------", index)
		for _, profile := range profiles {
			log.Errorf(profile.String())
		}
		log.Errorf("-------------------------------------------------------------------------------")
	}

	if len(pprof) > 0 {
		for i, profile := range pprof {
			show(i, profile)
		}
		return
	} else {
		show(0, profileListMap)
	}
}

func ShowDiffCacheCost(databaseCost, memoryCost map[string]*Profile) {

	profiles := make([]*Profile, 0, len(databaseCost))
	for _, profile := range databaseCost {
		profiles = append(profiles, profile)
	}

	slices.SortFunc(profiles, func(a, b *Profile) int {
		if a.TotalTime < b.TotalTime {
			return 1
		} else if a.TotalTime > b.TotalTime {
			return -1
		}
		return 0
	})

	for _, database := range profiles {
		key := database.Name
		memory, memory_have := memoryCost[key]
		if !memory_have {
			log.Errorf("Profile [%s] not found in memory cost", key)
			log.Error(database.String())
			continue
		}

		if database.Count > memory.Count*5 {
			log.Errorf("Profile [%s] count mismatch: database %d, memory %d", key, database.Count, memory.Count)
			log.Error(database.String())
			log.Error(memory.String())
		}

		if database.TotalTime > memory.TotalTime*2 {
			log.Errorf("------------------------------------------------------")
			log.Errorf("Profile [%s] total time mismatch: database %v, memory %v", key, time.Duration(database.TotalTime), time.Duration(memory.TotalTime))
			for index, databaseTime := range database.Times {
				if index >= len(memory.Times) {
					log.Errorf("Profile %s time mismatch at index %d: database %v, memory not found", key, index, time.Duration(databaseTime))
					log.Errorf("%s-%-4d\t database Time: %v\tConut: %v\t Avg: %v",
						key, index+1,
						time.Duration(databaseTime),
						database.Count,
						time.Duration(databaseTime)/time.Duration(database.Count),
					)
					continue
				}

				memoryTime := memory.Times[index]
				if databaseTime > memoryTime*2 || databaseTime > uint64(1*time.Second) {
					log.Errorf("Profile %s time mismatch at index %d: database %v, memory %v", key, index, time.Duration(databaseTime), time.Duration(memory.Times[index]))
					log.Errorf("%s-%-4d\t database Time: %v\tCount: %v\tAvg: %v",
						key, index+1,
						time.Duration(databaseTime),
						database.Count,
						time.Duration(databaseTime)/time.Duration(database.Count),
					)
					log.Errorf("%s-%-4d\t memory  Time: %v\tCount: %v\t Avg: %v",
						key, index+1,
						time.Duration(memoryTime),
						memory.Count,
						time.Duration(memoryTime)/time.Duration(memory.Count),
					)
				}
			}
		}

	}
}
