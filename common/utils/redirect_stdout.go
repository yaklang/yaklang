package utils

import (
	"context"
	"github.com/hpcloud/tail"
	uuid2 "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/log"
	"io/ioutil"
	"os"
	"path/filepath"
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
func HandleStdout(ctx context.Context, handle func(string)) error {
	if isInAttached.IsSet() {
		uuid := uuid2.NewV4().String()
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
