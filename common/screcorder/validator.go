package screcorder

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func GetDarwinAvailableAVFoundationScreenDevices() []*ScreenDevice {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	ffmpeg := consts.GetFfmpegPath()
	var raw, _ = exec.CommandContext(ctx, ffmpeg,
		"-f", "avfoundation",
		"-list_devices", "true",
		"-i", "").CombinedOutput()
	defer cancel()

	var availableScreenDevices []*ScreenDevice
	for _, i := range parseDarwinAVFoundationListDevices(string(raw)) {
		if strings.Contains(i.DeviceName, "screen") {
			availableScreenDevices = append(availableScreenDevices, i)
		}
	}
	if len(availableScreenDevices) > 0 {
		return availableScreenDevices
	}
	return nil
}

func IsAvailable() (bool, error) {
	path := consts.GetFfmpegPath()
	if path == "" {
		return false, utils.Error("ffmpeg is not existed in your os")
	}

	consts.GetGormCVEDatabase()
	switch runtime.GOOS {
	case "darwin":
		path, err := exec.LookPath(path)
		if err != nil {
			return false, utils.Errorf("cannot find executable item[%s] failed: %v", path, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		raw, err := exec.CommandContext(ctx, path, "-h").CombinedOutput()
		if err != nil {
			return false, utils.Errorf("checking ffmpeg failed: %s", err)
		}
		ctx, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		raw, _ = exec.CommandContext(ctx, path,
			"-f", "avfoundation",
			"-list_devices", "true",
			"-i", "").CombinedOutput()

		var availableScreenDevices []*ScreenDevice
		for _, i := range parseDarwinAVFoundationListDevices(string(raw)) {
			log.Infof("checking devicename: %v", i.DeviceName)
			if strings.Contains(fmt.Sprint(i.DeviceName), "Capture screen") {
				availableScreenDevices = append(availableScreenDevices, i)
			}
		}
		if len(availableScreenDevices) > 0 {
			return true, nil
		}
		return false, utils.Errorf("cannot found screen devices")
	case "windows", "win32":
		ctx, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		raw, err := exec.CommandContext(ctx, path, "-h").CombinedOutput()
		if err != nil {
			return false, utils.Errorf("checking ffmpeg failed: %s", err)
		}
		_ = raw
		return true, nil
	default:
		return false, utils.Errorf("cannot support os: %v", runtime.GOOS)
	}
}
