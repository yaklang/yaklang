package fuzzx

import "github.com/yaklang/yaklang/common/mutate"

func (f *FuzzRequest) Exec(opts ...mutate.HttpPoolConfigOption) (chan *mutate.HttpResult, error) {
	return mutate.ExecPool(f.requests, opts...)
}

func (f *FuzzRequest) ExecFirst(opts ...mutate.HttpPoolConfigOption) (result *mutate.HttpResult, err error) {
	opts = append(opts, mutate.WithPoolOpt_RequestCountLimiter(1))
	ch, err := f.Exec(opts...)
	if err != nil {
		return nil, err
	}
	for r := range ch {
		result = r
	}
	if result == nil {
		return nil, new(NoResultError)
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return result, nil
}
