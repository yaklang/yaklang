package screcorder

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

/*
	# MacOS
	ffmpeg -f avfoundation -r 1.0 -i "0:" -vf "setpts=0.3333*PTS" -c:v libx264 -capture_cursor 1 output.flv

	# Windows
	ffmpeg -f gdigrab -framerate 1.0 -i desktop -c:v libx264 -preset ultrafast output.mp4
*/

type ScreenDevice struct {
	DeviceName string
	// in darwin: "1" "2" "3" is ok
	// linux: ":0.1" ":0.2" ":0.3"
	FfmpegInputName string
	PlatformDemuxer string
}

var darwinAVFoundationStripper = regexp.MustCompile(`\[AVFoundation indev @ 0x[0-9a-fA-F]{12}]\s+`)
var darwinAVFoundationScreenNameFetcher = regexp.MustCompile(`\[(\d+)]\s(.*)`)

func parseDarwinAVFoundationListDevices(raw string) []*ScreenDevice {
	var picked []*ScreenDevice

	var startToFetchScreen = false
	for line := range utils.ParseLines(raw) {
		line := strings.TrimSpace(line)
		if strings.HasPrefix(line, "[AVFoundation indev @ 0x") {
			line = strings.TrimSpace(darwinAVFoundationStripper.ReplaceAllString(line, ""))
			if strings.Contains(strings.ToLower(line), "avfoundation video devices") {
				startToFetchScreen = true
				continue
			}

			if strings.HasPrefix(strings.ToLower(line), "avfoundation") {
				startToFetchScreen = false
				continue
			}

			if startToFetchScreen {
				if ret := darwinAVFoundationScreenNameFetcher.FindStringSubmatch(line); len(ret) > 2 {
					deviceName := ret[1]
					deviceNameVerbose := ret[2]
					picked = append(picked, &ScreenDevice{
						DeviceName:      deviceNameVerbose,
						FfmpegInputName: deviceName,
						PlatformDemuxer: "avfoundation",
					})
				}
			}
		}
	}
	return picked
}
