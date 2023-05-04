package bruteutils

import (
	"bufio"
	"bytes"
	"container/list"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/mixer"
	"io/ioutil"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type BruteItem struct {
	Type     string
	Target   string
	Username string
	Password string
}

func (b *BruteItem) Result() *BruteItemResult {
	return &BruteItemResult{
		Type:             b.Type,
		Ok:               false,
		Finished:         false,
		UserEliminated:   false,
		OnlyNeedPassword: false,
		Target:           b.Target,
		Username:         b.Username,
		Password:         b.Password,
	}
}

func (b *BruteItem) String() string {
	return fmt.Sprintf("%s:%s@%v", b.Username, b.Password, b.Target)
}

type targetProcessing struct {
	Target   string
	Swg      *utils.SizedWaitGroup
	count    int32
	Items    []*BruteItem
	Finished bool
}

func (t *targetProcessing) GetCurrentCount() int {
	return int(atomic.LoadInt32(&t.count))
}

func (t *targetProcessing) Finish() {
	t.Finished = true
}

type BruteUtil struct {
	processes            sync.Map
	TargetTaskConcurrent int

	targetsSwg utils.SizedWaitGroup

	targetList     *list.List
	targetListLock sync.Mutex

	delayer *utils.DelayWaiter

	// 爆破任务的 callbacks
	callback BruteCallback

	// 每一个执行结束的结果回掉
	resultCallback BruteItemResultCallback

	// 这个选项标志着，如果遇到了 Ok，则停止对当前目标的爆破
	OkToStop bool

	// 完成阈值，这是一个整型
	// 在爆破过程中会统计任务 Finished 的数量
	// 一旦任务执行给的结果 Finished 的数量达到这个参数设置的值
	// 马上结束对当前这个目标的爆破
	FinishingThreshold int

	// OnlyNeedPassword 标志着这次爆破只需要密码进行爆破
	OnlyNeedPassword bool

	//
	beforeBruteCallback func(string) bool
}

func (b *BruteUtil) SetResultCallback(cb BruteItemResultCallback) {
	b.resultCallback = cb
}

type BruteItemResult struct {
	// 爆破类型
	Type string

	// 标志着爆破成功
	Ok bool

	// 标志着完成爆破/因为协议不对，或者是网络验证错误，等
	Finished bool

	// 标志着该用户名有问题，不应该再使用这个用户名
	UserEliminated bool

	// 该爆破只需要密码，不需要用户名
	OnlyNeedPassword bool

	// 爆破的目标
	Target string

	// 爆破的用户名与密码
	Username string
	Password string
}

func (r *BruteItemResult) String() string {
	var result = "FAIL"
	if r.Ok {
		result = "OK  "
	} else {
		result = "FAIL"
	}
	return fmt.Sprintf("[%v]: %v:\\\\%v:%v@%v", result, r.Type, r.Username, r.Password, r.Target)
}

func (r *BruteItemResult) Show() {
	println(r.String())
}

type BruteCallback func(item *BruteItem) *BruteItemResult
type BruteItemResultCallback func(b *BruteItemResult)

func NewMultiTargetBruteUtil(targetsConcurrent, minDelay, maxDelay int, callback BruteCallback) (*BruteUtil, error) {
	delayer, err := utils.NewDelayWaiter(int32(minDelay), int32(maxDelay))
	if err != nil {
		return nil, errors.Errorf("create delayer failed: %s", err)
	}
	// first is 0 delay
	delayer.Wait()
	return &BruteUtil{
		TargetTaskConcurrent: 1,
		targetList:           list.New(),
		targetsSwg:           utils.NewSizedWaitGroup(targetsConcurrent),
		callback:             callback,
		delayer:              delayer,
	}, nil
}

func (b *BruteUtil) Feed(item *BruteItem) {
	process, err := b.GetProcessingByTarget(item.Target)
	if err != nil {
		// new target
		swg := utils.NewSizedWaitGroup(b.TargetTaskConcurrent)
		process = &targetProcessing{
			Target: item.Target,
			Swg:    &swg,
		}
		b.targetList.PushBack(item.Target)
		b.processes.Store(item.Target, process)
	}

	process.Items = append(process.Items, item)
}

func (b *BruteUtil) GetProcessingByTarget(target string) (*targetProcessing, error) {
	if raw, ok := b.processes.Load(target); ok {
		return raw.(*targetProcessing), nil
	} else {
		return nil, errors.New("no such target")
	}
}

func (b *BruteUtil) GetAllTargetsProcessing() []*targetProcessing {
	var ct []*targetProcessing
	b.processes.Range(func(key, value interface{}) bool {
		p := value.(*targetProcessing)
		ct = append(ct, p)
		return true
	})
	return ct
}

func (b *BruteUtil) RemoteProcessingByTarget(target string) {
	b.processes.Delete(target)
}

func (b *BruteUtil) Run() error {
	return b.run(context.Background())
}

func (b *BruteUtil) RunWithContext(ctx context.Context) error {
	return b.run(ctx)
}

func (b *BruteUtil) run(ctx context.Context) error {
	defer b.targetsSwg.Wait()

	for {
		target, err := b.popFirstTarget()
		if err != nil {
			log.Trace("finished poping target from target list")
			break
		}

		if target == "" {
			continue
		}

		// context cancel
		if err := ctx.Err(); err != nil {
			log.Info("context canceled")
			return errors.New("user canceled")
		}

		err = b.targetsSwg.AddWithContext(ctx)
		if err != nil {
			break
		}

		go func(t string) {
			defer b.targetsSwg.Done()

			log.Tracef("start processing for target: %s", t)
			err := b.startProcessingTarget(t, ctx)
			if err != nil {
				log.Errorf("start processing brute target failed: %s", err)
			}
		}(target)
	}
	return nil
}

func (b *BruteUtil) startProcessingTarget(target string, ctx context.Context) error {
	defer func() {
		go func() {
			select {
			case <-time.NewTimer(5 * time.Second).C:
				b.RemoteProcessingByTarget(target)
			}
		}()
	}()

	process, err := b.GetProcessingByTarget(target)
	if err != nil {
		return errors.Errorf("start processing target failed: %s", err)
	}
	defer func() {
		process.Swg.Wait()
		process.Finish()
	}()

	var (
		finishedCount    int32 = 0
		finished               = utils.NewBool(false)
		onlyNeedPassword       = utils.NewBool(b.OnlyNeedPassword)
		eliminatedUsers        = sync.Map{}
		usedPassword           = sync.Map{}
	)

	// 做爆破前的检查，检查目标合理性，如果不合理，马上结束
	// 通常包含如下部分：
	//    1. 检查目标合理性
	//    2. 检查目标指纹
	if b.beforeBruteCallback != nil {
		if !b.beforeBruteCallback(target) {
			return errors.Errorf("pre-checking target[%s] failed", target)
		}
	}

	for _, i := range process.Items {
		if err := ctx.Err(); err != nil {
			return errors.New("context canceled")
		}

		// 退出爆破
		if finished.IsSet() {
			break
		}

		// 计算子任务要求退出爆破次数
		if atomic.LoadInt32(&finishedCount) >= int32(b.FinishingThreshold) && b.FinishingThreshold != 0 {
			break
		}

		// 如果该爆破只要求密码不要求用户名
		if onlyNeedPassword.IsSet() {
			if _, ok := usedPassword.Load(i.Password); ok {
				// 如果这个密码已经被用过了，就马上进入下一组
				continue
			} else {
				// 如果这个密码没有被用过，则记录该密码，并下一个
				usedPassword.Store(i.Password, 1)
			}
		}

		err := process.Swg.AddWithContext(ctx)
		if err != nil {
			return nil
		}

		i := i
		go func(item *BruteItem) {
			defer func() {
				process.Swg.Done()
				atomic.AddInt32(&process.count, 1)
			}()

			// 废弃的用户名
			if _, ok := eliminatedUsers.Load(item.Username); ok {
				// 如果该用户名是被丢弃的，则应该直接不启动该任务的爆破
				return
			}

			// 执行爆破函数
			result := b.callback(item)
			if result == nil {
				return
			}

			if b.resultCallback != nil {
				b.resultCallback(result)
			}

			// 是否遇到了爆破成功的情况？
			if result.Ok && b.OkToStop {
				finished.Set()
			}

			// 是否当前结果是完成？
			if result.Finished {
				atomic.AddInt32(&finishedCount, 1)
			}

			// 是否有结果发现这个目标是只需要密码的
			if result.OnlyNeedPassword {
				onlyNeedPassword.Set()
			}

			// 确定当前用户名已经是废掉的用户名，对当前目标不再使用当前这个用户名
			if result.UserEliminated {
				eliminatedUsers.Store(item.Username, 1)
			}
			b.delayer.Wait()
		}(i)
	}

	log.Tracef("finished handling target: %s", target)
	return nil
}

func (b *BruteUtil) popFirstTarget() (string, error) {
	b.targetListLock.Lock()
	defer b.targetListLock.Unlock()

	e := b.targetList.Front()
	if e == nil {
		return "", errors.New("emtpy targets")
	}

	defer func() {
		_ = b.targetList.Remove(e)
	}()

	return e.Value.(string), nil
}

// 使用更合理的接口来构建 BruteUtil

type OptionsAction func(util *BruteUtil)

// 这个选项控制整体的目标并发 默认值为 200
func WithTargetsConcurrent(targetsConcurrent int) OptionsAction {
	return func(util *BruteUtil) {
		util.targetsSwg = utils.NewSizedWaitGroup(targetsConcurrent)
	}
}

// 这个选项来控制每个目标最多同时执行多少个爆破任务，默认为 1
func WithTargetTasksConcurrent(targetTasksConcurrent int) OptionsAction {
	return func(util *BruteUtil) {
		util.TargetTaskConcurrent = targetTasksConcurrent
	}
}

// 这个选项来控制设置 Delayer
func WithDelayerWaiter(minDelay, maxDelay int) (OptionsAction, error) {
	dlr, err := utils.NewDelayWaiter(int32(minDelay), int32(maxDelay))
	if err != nil {
		return nil, errors.Errorf("delay waiter build failed: %s", err)
	}
	return func(util *BruteUtil) {
		util.delayer = dlr
	}, nil
}

// 设置爆破任务
func WithBruteCallback(callback BruteCallback) OptionsAction {
	return func(util *BruteUtil) {
		util.callback = callback
	}
}

// 设置结果回调
func WithResultCallback(callback BruteItemResultCallback) OptionsAction {
	return func(util *BruteUtil) {
		util.resultCallback = callback
	}
}

// 设置 OkToStop 选项
func WithOkToStop(t bool) OptionsAction {
	return func(util *BruteUtil) {
		util.OkToStop = t
	}
}

// 设置阈值
func WithFinishingThreshold(t int) OptionsAction {
	return func(util *BruteUtil) {
		util.FinishingThreshold = t
	}
}

// 设置只需要密码爆破
func WithOnlyNeedPassword(t bool) OptionsAction {
	return func(util *BruteUtil) {
		util.OnlyNeedPassword = t
	}
}

// 设置爆破预检查函数
func WithBeforeBruteCallback(c func(string) bool) OptionsAction {
	return func(util *BruteUtil) {
		util.beforeBruteCallback = c
	}
}

func NewMultiTargetBruteUtilEx(options ...OptionsAction) (*BruteUtil, error) {
	delayer, err := utils.NewDelayWaiter(0, 0)
	if err != nil {
		return nil, errors.Errorf("init delay waiter failed: %s", err)
	}

	bu := &BruteUtil{
		TargetTaskConcurrent: 1,
		targetsSwg:           utils.NewSizedWaitGroup(200),
		OkToStop:             false,
		FinishingThreshold:   0,
		OnlyNeedPassword:     false,
		delayer:              delayer,
		targetList:           list.New(),
	}

	for _, option := range options {
		option(bu)
	}

	if bu.callback == nil {
		return nil, errors.New("callback is not set")
	}
	return bu, nil
}

func BruteItemStreamWithContext(ctx context.Context, typeStr string, target []string, users []string, pass []string) (chan *BruteItem, error) {
	mixerIns, err := mixer.NewMixer(target, pass, users)
	if err != nil {
		return nil, utils.Errorf("create target/user/password mixer failed: %s", err)
	}

	ch := make(chan *BruteItem)
	go func() {
		defer close(ch)

		for {
			result := mixerIns.Value()
			select {
			case ch <- &BruteItem{
				Type:     typeStr,
				Target:   result[0],
				Password: result[1],
				Username: result[2],
			}:
			case <-ctx.Done():
				return
			}
			if err := mixerIns.Next(); err != nil {
				return
			}
		}
	}()
	return ch, nil
}

func FileOrMutateTemplateForStrings(divider string, t ...string) []string {
	var r []string
	for _, item := range t {
		r = append(r, FileOrMutateTemplate(item, divider)...)
	}
	return r
}

func FileOrMutateTemplate(t string, divider string) []string {
	targetList := FileToDictList(t)

	if targetList == nil {
		for _, user := range utils.PrettifyListFromStringSplited(t, divider) {
			_l, err := mutate.QuickMutate(user, nil)
			if err != nil {
				continue
			}
			targetList = append(targetList, _l...)
		}
	}

	if targetList == nil {
		targetList = append(targetList, t)
	}
	return targetList
}

func FileToDictList(fileName string) []string {
	fd, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Error(err)
		return nil
	}
	scanner := bufio.NewScanner(bytes.NewBuffer(fd))
	scanner.Split(bufio.ScanLines)

	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		lines = append(lines, line)
	}
	return lines
}

func (b *BruteUtil) StreamBruteContext(
	ctx context.Context, typeStr string, target, users, pass []string,
	resultCallback BruteItemResultCallback,
) error {
	ch, err := BruteItemStreamWithContext(ctx, typeStr, target, users, pass)
	if err != nil {
		return err
	}
	b.SetResultCallback(resultCallback)
	log.Infof("brute task with target[%v] user[%v] password[%v]", len(target), len(users), len(pass))
	for item := range ch {
		b.Feed(item)
	}
	err = b.RunWithContext(ctx)
	if err != nil {
		return err
	}
	return nil
}

func autoSetFinishedByConnectionError(err error, result *BruteItemResult) *BruteItemResult {
	switch true {
	case utils.IContains(err.Error(), "connect: connection refused"):
		fallthrough
	case utils.IContains(err.Error(), "no pg_hba.conf entry for host"):
		fallthrough
	case utils.IContains(err.Error(), "network unreachable"):
		fallthrough
	case utils.IContains(err.Error(), "network is unreachable"):
		fallthrough
	//case utils.IContains(err.Error(), "remote error: tls: access denied"):
	//	fallthrough
	case utils.IContains(err.Error(), "no reachable servers"):
		fallthrough
	case utils.IContains(err.Error(), "i/o timeout"):
		result.Finished = true
		return result
	default:
		log.Error(err.Error())
		return result
	}
}
