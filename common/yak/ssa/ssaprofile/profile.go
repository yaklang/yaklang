package ssaprofile

import (
	"fmt"
	"slices"
	"strings"
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
		return "Profile [nil]"
	}

	var builder strings.Builder
	totalDur := time.Duration(profile.TotalTime)
	count := profile.Count
	var avg time.Duration
	if count > 0 {
		avg = totalDur / time.Duration(count)
	}

	builder.WriteString(fmt.Sprintf("----------- ProfileList [%s] --------------------\n", profile.Name))
	builder.WriteString(fmt.Sprintf("-------- Profile %s\tCount %v\n", profile.Name, profile.Count))
	if count == 0 {
		return builder.String()
	}

	builder.WriteString(fmt.Sprintf("%s--all\tTime: %v\tCount: %v\tAvg: %v\n",
		profile.Name, totalDur, count, avg,
	))

	for index, t := range profile.Times {
		stepDur := time.Duration(t)
		stepAvg := stepDur
		if count > 0 {
			stepAvg = stepDur / time.Duration(count)
		}
		builder.WriteString(fmt.Sprintf("%s-%-4d\tTime: %v\tCount: %v\tAvg: %v\n",
			profile.Name, index+1, stepDur, count, stepAvg,
		))
	}
	return builder.String()
}

var profileListMap = utils.NewSafeMap[*Profile]()

func GetProfileListMap() *utils.SafeMap[*Profile] {
	return profileListMap
}
func Refresh() {
	profileListMap = utils.NewSafeMap[*Profile]()
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

func ProfileAddToMap(profileMap *utils.SafeMap[*Profile], enable bool, name string, fs ...func()) {
	fsWithErr := lo.FilterMap(fs, func(f func(), _ int) (func() error, bool) {
		if f == nil {
			return nil, false
		}
		return func() error {
			f()
			return nil
		}, true
	})
	ProfileAddWithErrorToMap(profileMap, enable, name, fsWithErr...)
}

func ProfileAddWithError(enable bool, name string, fs ...func() error) error {
	return ProfileAddWithErrorToMap(profileListMap, enable, name, fs...)
}

func ProfileAddWithErrorToMap(profileMap *utils.SafeMap[*Profile], enable bool, name string, fs ...func() error) error {
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
		p, ok = profileMap.Get(name)
		if !ok {
			p = &Profile{
				Name:       name,
				TotalTime:  0,
				Count:      0,
				ErrorCount: 0,
				Times:      make([]uint64, 0, len(fs)),
			}
			profileMap.Set(name, p)
		}
	}

	var total uint64 = 0
	for index, f := range fs {
		start := time.Now()
		if f != nil {
			if err := f(); err != nil {
				log.Debugf("ProfileAdd %s error: %v", name, err)
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

// ShowCompileProfiles 输出编译阶段的性能统计
func ShowCompileProfiles() {
	ShowProfileMaps("compile", profileListMap)
}

// ShowProfileMaps 按标签输出给定 Profile map 的统计信息
func ShowProfileMaps(label string, profileMaps ...*utils.SafeMap[*Profile]) {
	if len(profileMaps) == 0 {
		dumpProfileMap(label, profileListMap)
		return
	}
	for _, prof := range profileMaps {
		dumpProfileMap(label, prof)
	}
}

func dumpProfileMap(label string, prof *utils.SafeMap[*Profile]) {
	if prof == nil || prof.Count() == 0 {
		log.Infof("profile map %s is empty", label)
		return
	}

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
	log.Infof("========================================")
	log.Infof("Profile Summary [%s]", label)
	log.Infof("========================================")
	for _, profile := range profiles {
		log.Infof(profile.String())
	}
	log.Infof("========================================")
}

// ShowScanPerformance 输出代码扫描相关的性能日志
func ShowScanPerformance(ruleProfileMap *utils.SafeMap[*Profile], enableRulePerformance bool, totalDuration time.Duration) {
	ShowCompileProfiles()
	totalCount := uint64(0)
	if ruleProfileMap != nil {
		ruleProfileMap.ForEach(func(key string, value *Profile) bool {
			totalCount += value.Count
			return true
		})
	}
	if totalCount == 0 {
		totalCount = 1
	}
	avgDuration := totalDuration / time.Duration(totalCount)
	log.Infof("=== Scan Total ===")
	log.Infof("Time: %v\tCount: %d\tAvg: %v", totalDuration, totalCount, avgDuration)
	log.Infof("==================")
	if enableRulePerformance && ruleProfileMap != nil && ruleProfileMap.Count() > 0 {
		profiles := make([]*Profile, 0, ruleProfileMap.Count())
		ruleProfileMap.ForEach(func(key string, value *Profile) bool {
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
		log.Infof("=== Rule Performance (scan) ===")
		for _, profile := range profiles {
			if profile.Count == 0 {
				continue
			}
			avg := time.Duration(profile.TotalTime) / time.Duration(profile.Count)
			log.Infof("%s Time: %v Count: %d Avg: %v", profile.Name, time.Duration(profile.TotalTime), profile.Count, avg)
		}
		log.Infof("================================")
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
			log.Debugf("Profile [%s] not found in memory cost", key)
			log.Debug(database.String())
			continue
		}

		if database.Count > memory.Count*5 {
			log.Debugf("Profile [%s] count mismatch: database %d, memory %d", key, database.Count, memory.Count)
			log.Debug(database.String())
			log.Debug(memory.String())
		}

		if database.TotalTime > memory.TotalTime*2 {
			log.Debugf("------------------------------------------------------")
			log.Debugf("Profile [%s] total time mismatch: database %v, memory %v", key, time.Duration(database.TotalTime), time.Duration(memory.TotalTime))
			for index, databaseTime := range database.Times {
				if index >= len(memory.Times) {
					log.Debugf("Profile %s time mismatch at index %d: database %v, memory not found", key, index, time.Duration(databaseTime))
					log.Debugf("%s-%-4d\t database Time: %v\tConut: %v\t Avg: %v",
						key, index+1,
						time.Duration(databaseTime),
						database.Count,
						time.Duration(databaseTime)/time.Duration(database.Count),
					)
					continue
				}

				memoryTime := memory.Times[index]
				if databaseTime > memoryTime*2 || databaseTime > uint64(1*time.Second) {
					log.Debugf("Profile %s time mismatch at index %d: database %v, memory %v", key, index, time.Duration(databaseTime), time.Duration(memory.Times[index]))
					log.Debugf("%s-%-4d\t database Time: %v\tCount: %v\tAvg: %v",
						key, index+1,
						time.Duration(databaseTime),
						database.Count,
						time.Duration(databaseTime)/time.Duration(database.Count),
					)
					log.Debugf("%s-%-4d\t memory  Time: %v\tCount: %v\t Avg: %v",
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
