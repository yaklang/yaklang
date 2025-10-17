package screcorder

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
)

func TestFFmpeg_List(t *testing.T) {
	raw := `[AVFoundation indev @ 0x7fa5c09046c0] AVFoundation video devices:
[AVFoundation indev @ 0x7fa5c09046c0] [0] Capture screen 0
[AVFoundation indev @ 0x7fa5c09046c0] AVFoundation audio devices:
[AVFoundation indev @ 0x7fa5c09046c0] [0] V1ll4n 的AirPods Max
[AVFoundation indev @ 0x7fa5c09046c0] [1] ByteviewAudioDevice`
	ret := parseDarwinAVFoundationListDevices(raw)
	spew.Dump(ret)
	if len(ret) != 1 {
		panic("parse error")
	}
	if ret[0].FfmpegInputName != "0" {
		panic("device error")
	}
}

func TestRecorder_Start(t *testing.T) {
	devices := GetDarwinAvailableAVFoundationScreenDevices()
	if len(devices) == 0 {
		t.Skip("no screen device found")
		return
	}
	device := devices[0]

	recorder, err := NewScreenRecorder(nil, device)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close()

	err = recorder.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)

	recorder.Stop()

	t.Log("record file:", recorder.Filename())
	_, err = os.Stat(recorder.Filename())
	if err != nil {
		t.Fatal(err)
	}
}
