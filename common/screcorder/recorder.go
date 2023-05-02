package screcorder

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"
	"yaklang.io/yaklang/common/consts"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
)

type Recorder struct {
	conf       *Config
	started    *utils.AtomicBool
	running    *utils.AtomicBool
	files      []string
	onFileSave func(string)
	fileMutex  sync.Mutex
}

func (r *Recorder) IsRunning() bool {
	return r.running.IsSet()
}

func (r *Recorder) Start(outputName string) error {
	if r.started.IsSet() {
		return utils.Error("this recorder is started")
	}

	r.started.Set()
	switch runtime.GOOS {
	case "darwin":
		var baseName string
		var suffix = ".mp4"
		if ret := path.Ext(outputName); ret != "" {
			suffix = ret
			baseName = outputName[:len(outputName)-len(ret)]
		} else {
			baseName = outputName
		}
		dvs := GetDarwinAvailableAVFoundationScreenDevices()
		if len(dvs) <= 0 {
			return utils.Error("get available avfoundation screen dev failed")
		}
		var paramsSet = make(map[string][]string)
		for index, dev := range dvs {
			dev := dev
			indexStr := fmt.Sprint(index)
			if indexStr == "0" || indexStr == "_0" {
				indexStr = ""
			}
			var outputFilename = baseName + indexStr + suffix
			input := strings.TrimSuffix(dev.FfmpegInputName, ":") + ":"

			if utils.GetFirstExistedFile(outputFilename) != "" {
				return utils.Errorf("file: %v is existed", outputFilename)
			}

			params, err := r.conf.ToParams(input, outputFilename)
			if err != nil {
				return err
			}
			paramsSet[outputFilename] = params
		}

		if len(paramsSet) <= 0 {
			return utils.Error("get params set failed")
		}

		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		wg.Add(len(paramsSet))
		var writers []io.Writer
		r.running.UnSet()
		for outputFile, p := range paramsSet {
			outputFileName := outputFile
			params := p
			cmd := exec.CommandContext(ctx, consts.GetFfmpegPath(), params...)
			writer, err := cmd.StdinPipe()
			if err != nil {
				cancel()
				return utils.Error("get stdin pipe failed: " + err.Error())
			}
			writers = append(writers, writer)
			//cmd.Stdout = os.Stdout
			//cmd.Stderr = os.Stderr
			go func() {
				defer wg.Done()
				defer func() {
					if err := recover(); err != nil {
						log.Warnf("run failed: %s", err)
					}
				}()
				err := cmd.Run()
				if err != nil {
					log.Errorf("run failed: %s", err)
				}

				if utils.GetFirstExistedFile(outputFileName) != "" {
					r.appendFile(outputFileName)
				}
			}()
		}
		go func() {
			r.running.Set()
			wg.Wait()
			r.running.IsSet()
		}()
		go func() {
			defer func() {
				cancel()
				if err := recover(); err != nil {
					log.Warnf("run failed: %s", err)
				}
			}()
			select {
			case <-r.conf.ctx.Done():
				log.Info("recording is finished!")
			}
			for _, w := range writers {
				log.Info("start to write 'q' ")
				w.Write([]byte{'q', 'q', 'q'})
			}
			r.running.UnSet()
			time.Sleep(10 * time.Second)
		}()
	case "windows":
		if path.Ext(outputName) == "" {
			outputName += ".mp4"
		}
		extraParams, err := r.conf.ToParams("desktop", outputName)
		if err != nil {
			return err
		}
		if utils.GetFirstExistedFile(outputName) != "" {
			return utils.Errorf("file{%v} is existed", outputName)
		}
		procCtx, procKill := context.WithCancel(context.Background())
		var cmd = exec.CommandContext(procCtx, consts.GetFfmpegPath(), extraParams...)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			procKill()
			return utils.Errorf("create stdin pip failed: %s", err)
		}
		go func() {
			defer func() {
				if err := recover(); err != nil {
					log.Warnf("exit normally failed: %s", err)
				}
			}()
			select {
			case <-r.conf.ctx.Done():
			}
			stdin.Write([]byte{'q'})
			time.Sleep(10 * time.Second)
		}()

		go func() {
			defer func() {
				procKill()
			}()
			log.Infof("calling ffmpeg %v", strings.Join(extraParams, " "))
			r.running.Set()
			err := cmd.Run()
			if err != nil {
				log.Errorf("exec ffmpeg failed: %s", err)
			}
			if utils.GetFirstExistedFile(outputName) != "" {
				r.appendFile(outputName)
			}
			r.running.UnSet()
		}()
	}
	return nil
}

func (r *Recorder) Stop() {
	if !r.started.IsSet() {
		return
	}

	if r.conf.cancel != nil {
		r.conf.cancel()
	}
}

func (r *Recorder) OutputFiles() []string {
	return r.files[:]
}

func (r *Recorder) OnFileAppended(h func(i string)) {
	r.onFileSave = h
}

func (r *Recorder) appendFile(f string) {
	r.fileMutex.Lock()
	defer r.fileMutex.Unlock()

	r.files = append(r.files, f)
	if r.onFileSave != nil {
		r.onFileSave(f)
	}
}

func NewRecorder(opt ...ConfigOpt) *Recorder {
	c := NewDefaultConfig()
	for _, i := range opt {
		i(c)
	}
	return &Recorder{conf: c, started: utils.NewBool(false), running: utils.NewBool(true)}
}
