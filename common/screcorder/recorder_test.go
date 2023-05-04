package screcorder

import (
	"github.com/davecgh/go-spew/spew"
	"os"
	"testing"
	"time"
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
	os.Remove("/tmp/abc.mp4")
	recorder := NewRecorder()
	err := recorder.Start("/tmp/abc.mp4")
	if err != nil {
		panic(err)
	}
	time.Sleep(3 * time.Second)
	recorder.Stop()
	time.Sleep(2 * time.Second)
	spew.Dump(recorder.OutputFiles())
}
