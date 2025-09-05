package ssaprofile

import (
	"fmt"
	"slices"
	syncAtomic "sync/atomic"
	"time"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type Profile struct {
	Name       string
	TotalTime  uint64
	Count      uint64
	ErrorCount uint64
	Times      []uint64
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

var profileListMap = utils.NewSafeMap[*Profile]()

func GetProfileListMap() *utils.SafeMap[*Profile] {
	return profileListMap
}
func Refresh() {
	profileListMap.Clear()
}

func ProfileAdd(enable bool, name string, fs ...func()) {
	fsWithErr := lo.FilterMap(fs, func(f func(), _ int) (func() error, bool) {
		if f == nil {
			return nil, false
		}
		return func() error {
			f()
			return nil
		}, true
	})
	ProfileAddWithError(enable, name, fsWithErr...)
}

func ProfileAddWithError(enable bool, name string, fs ...func() error) error {
	defer func() {
		if err := recover(); err != nil {
			log.Infof("err: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	if name == "" {
		return fmt.Errorf("ProfileAdd name is empty")
	}

	var p *Profile
	if enable {
		var ok bool
		p, ok = profileListMap.Get(name)
		if !ok {
			p = &Profile{
				Name:       name,
				TotalTime:  0,
				Count:      0,
				ErrorCount: 0,
				Times:      make([]uint64, 0, len(fs)),
			}
			profileListMap.Set(name, p)
		}
	}

	var total uint64 = 0
	for index, f := range fs {
		start := time.Now()
		if f != nil {
			if err := f(); err != nil {
				log.Errorf("ProfileAdd %s error: %v", name, err)
				if p != nil {
					syncAtomic.AddUint64(&p.ErrorCount, 1)
				}
				return err
			}
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
	}
	return nil
}

func ShowCacheCost(pprof ...*utils.SafeMap[*Profile]) {

	show := func(index int, prof *utils.SafeMap[*Profile]) {
		profiles := make([]*Profile, 0, prof.Count())
		prof.ForEach(func(key string, value *Profile) bool {
			profiles = append(profiles, value)
			return true
		})

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

func ShowDiffCacheCost(databaseCost, memoryCost *utils.SafeMap[*Profile]) {

	profiles := make([]*Profile, 0, databaseCost.Count())
	databaseCost.ForEach(func(key string, profile *Profile) bool {
		profiles = append(profiles, profile)
		return true
	})

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
		memory, memory_have := memoryCost.Get(key)
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
