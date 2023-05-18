package screcorder

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"github.com/alfg/mp4"
	"github.com/disintegration/imaging"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"image/jpeg"
	"io"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"
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

func VideoCoverBase64(fileName string)  (imgBase64 string, err error) {
	reader := ExampleReadFrameAsJpeg(fileName, 1)
	img, err := imaging.Decode(reader)
	if err != nil {
		return "", err
	}
	buffer := &bytes.Buffer{}
	err = jpeg.Encode(buffer, img, nil)
	if err != nil {
		return "", utils.Errorf("failed: %s", err)
	}
	imgData := buffer.Bytes()

	// 将图像字节数组转换为 Base64 编码的字符串
	base64Image := base64.StdEncoding.EncodeToString(imgData)
	return base64Image, nil
}

func ExampleReadFrameAsJpeg(inFileName string, frameNum int) io.Reader {
	cmd := exec.CommandContext(context.Background(), consts.GetFfmpegPath(), "-i", inFileName,  "-vf", fmt.Sprintf("select=gte(n\\,%d),scale=-1:600", frameNum), "-frames:v", "1", "-f", "image2", "-codec:v", "mjpeg", "pipe:1")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil
	}
	return &out
}

func VideoDuration(path string) string {
	file, err := os.OpenFile(path, os.O_APPEND, 0777)
	if err != nil {
		return "0"
	}
	defer file.Close()
	mp4info, err := mp4.Open(path)
	moov := mp4info.Moov
	if moov == nil {
		return "0"
	}
	mvhd := moov.Mvhd
	if mvhd == nil {
		return "0"
	}
	duration := mvhd.Duration
	if duration > 0 {
		timeFormat := time.Duration(duration) * time.Millisecond
		h := int(timeFormat.Hours())
		m := int(timeFormat.Minutes()) % 60
		s := int(timeFormat.Seconds()) % 60
		time := fmt.Sprintf("%02d:%02d:%02d", h, m, s)
		return time
	}
	return "0"
}