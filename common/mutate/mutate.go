package mutate

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/jinzhu/gorm"
)

type RegexpMutateCondition struct {
	Verbose string
	// Regexp  *regexp.Regexp
	TagName string
	Handle  func(db *gorm.DB, s string) ([]string, error)
}

type MutateResult struct {
	Result   string
	Payloads []string
}

func QuickMutate(origin string, db *gorm.DB, conds ...*RegexpMutateCondition) ([]string, error) {
	if origin == "" {
		return []string{""}, nil
	}
	res, err := QuickMutateEx(origin, db, conds...)
	if err != nil {
		return nil, err
	}

	var z []string
	for _, r := range res {
		z = append(z, r.Result)
	}
	return utils.RemoveRepeatStringSlice(z), nil
}

func QuickMutateEx(origin string, db *gorm.DB, conds ...*RegexpMutateCondition) ([]*MutateResult, error) {
	return QuickMutateWithCallbackEx(origin, db, nil, conds...)
}

func QuickMutateWithCallbackEx(origin string, db *gorm.DB, callbacks []func(result *MutateResult), conds ...*RegexpMutateCondition) ([]*MutateResult, error) {
	var cbs []func(result *MutateResult) bool
	for _, c := range callbacks {
		cbs = append(cbs, func(result *MutateResult) bool {
			c(result)
			return true
		})
	}
	return QuickMutateWithCallbackEx2(origin, db, cbs, conds...)
}

func regexpMutateConditionToFuzzTagHandler(conds ...*RegexpMutateCondition) []FuzzConfigOpt {
	var opts []FuzzConfigOpt
	for _, c := range conds {
		if c == nil {
			continue
		}

		c := c
		opts = append(opts, Fuzz_WithExtraFuzzTagHandler(c.TagName, func(s string) (finalResult []string) {
			defer func() {
				if err := recover(); err != nil {
					log.Error(err)
					finalResult = []string{s}
				}
			}()

			results, err := c.Handle(consts.GetGormProfileDatabase(), s)
			if err != nil {
				panic(err)
			}
			return results
		}))
	}
	return opts
}

func QuickMutateWithCallbackEx2(origin string, db *gorm.DB, callbacks []func(result *MutateResult) bool, conds ...*RegexpMutateCondition) ([]*MutateResult, error) {
	opts := regexpMutateConditionToFuzzTagHandler(conds...)
	var results []*MutateResult
	opts = append(opts, Fuzz_WithResultHandler(func(s string, i []string) bool {
		result := &MutateResult{Result: s, Payloads: i}
		results = append(results, result)
		if len(callbacks) <= 0 {
			return true
		}
		for _, cb := range callbacks {
			cb(result)
		}
		return true
	}))
	_, err := FuzzTagExec(origin, opts...)
	if err != nil {
		return nil, err
	}
	return results, nil
}
