package utils

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/hpcloud/tail"
	"github.com/yaklang/yaklang/common/log"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	isInAttached         = NewBool(false)
	isInCached           = NewBool(false)
	attachOutputCallback = new(sync.Map)
	cachedLog            *CircularQueue
)

func GetCachedLog() (res []string) {
	for _, e := range cachedLog.GetElements() {
		res = append(res, e.(string))
	}
	return
}
func StartCacheLog(ctx context.Context, n int) {
	cachedLog = NewCircularQueue(n)
	if isInCached.IsSet() {
		return
	}
	isInCached.Set()
	go func() {
		if err := HandleStdout(ctx, func(s string) {
			cachedLog.Push(s)
		}); err != nil {
			log.Error(err)
		}
		isInCached.UnSet()
	}()
}

func HandleStdoutBackgroundForTest(handle func(string)) (func(), func(), error) {
	ctx := context.Background()
	var l int32 = 0xffff - 0xfe00
	n := rand.Int31n(l)
	msg := string([]rune{n + 0xfe00})
	endCh := make(chan struct{})
	endFlagMsg := fmt.Sprintf("%s", msg)
	sendEndMsg := func() {
		println(endFlagMsg)
	}
	checkEndMsg := func(s string) {
		if strings.Contains(s, msg) {
			endCh <- struct{}{}
		}
	}
	waitEnd := func() {
		select {
		case <-endCh:
		case <-time.After(time.Second * 3):
		}
	}
	startCh := make(chan struct{})
	once := sync.Once{}
	var err error
	go func() {
		err = HandleStdout(ctx, func(s string) {
			once.Do(func() {
				startCh <- struct{}{}
			})
			handle(s)
			checkEndMsg(s)
		})
		once.Do(func() {
			startCh <- struct{}{}
		})
	}()
	for i := 0; i < 10; i++ {
		select {
		case <-startCh:
			return sendEndMsg, waitEnd, err
		case <-time.After(100 * time.Millisecond):
			fmt.Println("waiting for mirror stdout start signal...")
		}
	}
	return nil, nil, Errorf("wait for mirror stdout start signal timeout")
}
func HandleStdout(ctx context.Context, handle func(string)) error {
	if isInAttached.IsSet() {
		uuid := uuid.New().String()
		attachOutputCallback.Store(uuid, func(result string) {
			defer func() {
				if err := recover(); err != nil {
				}
			}()
			handle(result)
		})
		select {
		case <-ctx.Done():
			attachOutputCallback.Delete(uuid)
			return nil
		}
	} else {
		isInAttached.Set()
	}
	GetDefaultYakitBaseTempDir := func() string {
		if os.Getenv("YAKIT_HOME") != "" {
			dirName := filepath.Join(os.Getenv("YAKIT_HOME"), "temp")
			if b, _ := PathExists(dirName); !b {
				os.MkdirAll(dirName, 0777)
			}
			return dirName
		}

		a := filepath.Join(GetHomeDirDefault("."), "yakit-projects", "temp")
		if GetFirstExistedPath(a) == "" {
			_ = os.MkdirAll(a, 0777)
		}
		return a
	}
	tempOutputs, err := ioutil.TempFile(GetDefaultYakitBaseTempDir(), "combined-outputs-*.txt")
	if err != nil {
		return Errorf("create tempfile to buffer stdout&err failed: %s", err)
	}
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("tempfile sync panic: %s", err)
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(time.Second)
				tempOutputs.Sync()
			}
		}
	}()
	defer func() {
		os.RemoveAll(tempOutputs.Name())
	}()
	tailf, err := tail.TailFile(tempOutputs.Name(), tail.Config{
		MustExist: true,
		Follow:    true,
	})
	if err != nil {
		return Errorf("tail -f `%v` failed: %s", tempOutputs.Name(), err)
	}
	ctx, cancelCtx := context.WithCancel(ctx)
	defer func() {
		cancelCtx()
	}()

	sendOutput := func(result string) {
		handle(result)
		attachOutputCallback.Range(func(key, value any) bool {
			va, _ := value.(func(result string))
			if va != nil {
				va(result)
			}
			return true
		})
	}
	// 恢复标准错误与标准输出流
	originStdout := os.Stdout
	originStderr := os.Stderr
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("attached panic: %s", err)
			}
			isInAttached.UnSet()
		}()
		for {
			if tailf == nil {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case line, ok := <-tailf.Lines:
				if !ok {
					return
				}
				if line == nil {
					continue
				}
				originStdout.Write([]byte(line.Text + "\n"))
				sendOutput(line.Text)
			}
		}
	}()

	cancel := func() {
		os.Stdout = originStdout
		os.Stderr = originStderr
		log.SetOutput(os.Stdout)
	}
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("attach finished with panic: %s", err)
		}
		cancel()
	}()

	os.Stdout = tempOutputs
	os.Stderr = tempOutputs
	log.SetOutput(tempOutputs)
	log.DefaultLogger.Printer.IsTerminal = true
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}
