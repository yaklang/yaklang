package fp

import (
	"context"
	"github.com/pkg/errors"
	"sync"
	utils2 "yaklang.io/yaklang/common/utils"
)

type Pool struct {
	matcher *Matcher
	ctx     context.Context
	cancel  context.CancelFunc
	targets chan *PoolTask
	swg     *utils2.SizedWaitGroup

	callbacks []PoolCallback
}

type PoolTask struct {
	Host    string
	Port    int
	Urls    []string
	Options []ConfigOption
	ctx     context.Context
}

func (t *PoolTask) WithContext(ctx context.Context) *PoolTask {
	t.ctx = ctx
	return t
}

func NewExecutingPool(
	ctx context.Context,
	size int,
	targetStream chan *PoolTask,
	config *Config,
) (*Pool, error) {
	if config == nil {
		config = NewConfig()
	}

	pCtx, cancel := context.WithCancel(ctx)
	matcher, err := NewDefaultFingerprintMatcher(config)
	if err != nil {
		return nil, errors.Errorf("create notification")
	}

	swg := utils2.NewSizedWaitGroup(size)
	pool := &Pool{
		cancel:  cancel,
		matcher: matcher,
		targets: targetStream,
		ctx:     pCtx,
		swg:     &swg,
	}

	return pool, nil
}

var (
	callbackSliceMutex sync.Mutex
)

type PoolCallback func(matcherResult *MatchResult, err error)

func (p *Pool) AddCallback(cb PoolCallback) {
	callbackSliceMutex.Lock()
	defer callbackSliceMutex.Unlock()

	p.callbacks = append(p.callbacks, cb)
}

func (p *Pool) Submit(t *PoolTask, async bool) bool {
	if async {
		select {
		case p.targets <- t:
			return true
		default:
			return false
		}
	} else {
		select {
		case p.targets <- t:
			return true
		}
	}
}

func (p *Pool) Run() error {
DISPATCH:
	for {
		select {
		case target, ok := <-p.targets:
			if !ok {
				break DISPATCH
			}
			_ = target

			err := p.swg.AddWithContext(p.ctx)
			if err != nil {
				return errors.Errorf("swg add context failed: %s", err)
			}

			go func() {
				defer p.swg.Done()

				var matchCtx context.Context = p.ctx
				if target.ctx != nil {
					matchCtx = target.ctx
				}
				matchResult, err := p.matcher.MatchWithContext(matchCtx, target.Host, target.Port, target.Options...)
				callbackSliceMutex.Lock()
				defer callbackSliceMutex.Unlock()
				for _, cb := range p.callbacks {
					cb(matchResult, err)
				}
			}()
		}
	}

	p.swg.Wait()
	p.cancel()

	select {
	case <-p.ctx.Done():
	}

	return nil
}

func (p *Pool) Close() {
	p.cancel()
}
