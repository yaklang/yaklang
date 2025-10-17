package mediautils

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/whisperutils"
)

/*
go run common/yak/cmd/yak.go -c 'result = ffmpeg.ExtractAudioFromVideo("vtestdata/demo1.mp4")~; dump(result); srtname = whisper.ConvertAudioToSRTFile(result.FilePath)~; file.Cat(srtname); srt = whisper.CreateSRTManager(srtname)~; r = srt.GetSRTContextByOffsetSeconds(30, 10); dump(r)'
go run common/yak/cmd/yak.go -c 'result = ffmpeg.ExtractAudioFromVideo("vtestdata/demo1.mp4")~; dump(result); srtname = whisper.ConvertAudioToSRTFile(result.FilePath)~; file.Cat(srtname)'
*/

var WhisperExports = map[string]any{
	"ConvertAudioToSRTFile": _whisperConvertAudioToSRTFile,
	"CreateSRTManager":      whisperutils.NewSRTManagerFromFile,
}

// whisper.ConvertAudioToSRTFile can convert audio to srt file
//
// example:
// ```
// srtfilename = whisper.ConvertAudioToSRTFile("audio.mp3")~
// println(srtfilename)
// ```
func _whisperConvertAudioToSRTFile(i string) (string, error) {
	//  go run common/yak/cmd/yak.go -c 'result = ffmpeg.ExtractAudioFromVideo("vtestdata/demo1.mp4")~; dump(result); srtname = whisper.ConvertAudioToSRTFile(result.FilePath)~; file.Cat(srtname)'
	var opts []whisperutils.CliOption

	if consts.GetWhisperModelMediumPath() == "" {
		return "", utils.Errorf("fetch whisper model path failed, please set it in config.yaml")
	}
	opts = append(opts, whisperutils.WithModelPath(consts.GetWhisperModelMediumPath()))

	if consts.GetWhisperSileroVADPath() != "" {
		opts = append(opts,
			whisperutils.WithVAD(true),
			whisperutils.WithVADModelPath(consts.GetWhisperSileroVADPath()),
		)
	}

	_dir, name := filepath.Split(i)
	_ = _dir
	if name == "" {
		return "", utils.Errorf("cannot fetch filename from: %v", i)
	}
	prefix := filepath.Ext(name)
	nameWithoutExt := name[:len(name)-len(prefix)]
	outputFilename := filepath.Join(consts.GetDefaultYakitBaseTempDir(), fmt.Sprintf("%v_%v.srt", nameWithoutExt, utils.DatetimePretty2()))

	// 如果文件名被占用，尝试添加后缀 _1, _2, ..., _50
	if utils.FileExists(outputFilename) {
		baseFilename := filepath.Join(consts.GetDefaultYakitBaseTempDir(), fmt.Sprintf("%v_%v", nameWithoutExt, utils.DatetimePretty2()))
		found := false
		for i := 1; i <= 50; i++ {
			candidateFilename := fmt.Sprintf("%v_%d.srt", baseFilename, i)
			if !utils.FileExists(candidateFilename) {
				outputFilename = candidateFilename
				found = true
				break
			}
		}
		if !found {
			return "", utils.Errorf("cannot generate unique filename after 50 attempts, base: %v", baseFilename)
		}
	}

	result, err := whisperutils.InvokeWhisperCli(i, outputFilename, opts...)
	if err != nil {
		return "", utils.Errorf("call whisper-cli failed: %v", err)
	}
	for line := range result {
		log.Infof("recognized line %.02f->%.02f: %v", line.StartTime.Seconds(), line.EndTime.Seconds(), line.Text)
	}
	if !utils.FileExists(outputFilename) {
		return "", utils.Errorf("(utils.FileExists) srt output file is not created: %v", outputFilename)
	}
	if s, err := os.Stat(outputFilename); err != nil {
		return "", utils.Errorf("(os.Stat) srt output file is not created: %v", outputFilename)
	} else if s.Size() == 0 {
		return "", utils.Errorf("srt output file is empty: %v", outputFilename)
	}
	return outputFilename, nil
}

func ConvertMediaToSRTString(path string) (string, error) {
	srtFile, err := ConvertMediaToSRT(path)
	if err != nil {
		return "", err
	}
	srtContent, err := os.ReadFile(srtFile)
	if err != nil {
		return "", utils.Errorf("read srt file failed: %v", err)
	}
	return string(srtContent), nil
}

func ConvertMediaToSRT(path string) (string, error) {
	isVideo, err := utils.IsVideo(path)
	if err != nil {
		return "", err
	}
	audioPath := path
	if isVideo {
		ffmpegRes, err := _extractAudioFromVideo(path)
		if err != nil {
			return "", err
		}
		audioPath = ffmpegRes.FilePath
	}

	if ok, err := utils.IsAudio(audioPath); !ok || err != nil {
		return "", utils.Errorf("check audio file failed: %v", err)
	}

	srtFile, err := _whisperConvertAudioToSRTFile(audioPath)
	if err != nil {
		return "", utils.Errorf("convert audio to srt file failed: %v", err)
	}
	return srtFile, nil
}
