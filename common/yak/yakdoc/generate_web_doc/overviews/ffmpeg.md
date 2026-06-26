`ffmpeg` 库基于 ffmpeg 提供音视频处理能力，支持抽帧、抽音轨、切片、烧录字幕、屏幕截图等，常用于多媒体取证、视频内容分析与 AI 多模态预处理。

典型使用场景：

- 抽取：`ffmpeg.ExtractAudioFromVideo` 抽音轨，`ffmpeg.ExtractFineGrainedFramesFromVideo` / `ffmpeg.ExtractBroadGrainedFramesFromVideo` 按粒度抽帧，`ffmpeg.ExtractVideoSliceFromVideo` 切片，`ffmpeg.ExtractUserScreenshot` 截屏。
- 字幕：`ffmpeg.BurnSRTIntoVideo` 把 SRT 字幕烧录进视频。
- 选项：`ffmpeg.withStartEnd`（时间区间）、`ffmpeg.withSliceDurationSeconds` / `ffmpeg.withSliceTargetFPS`（切片参数）、`ffmpeg.withOutputFile` / `ffmpeg.withSliceOutputDir`（输出）等。

与相邻库的关系：`ffmpeg` 是多媒体预处理工具，抽出的帧/音频常交给 `whisper`（语音转写）、`imageutils`（图像处理）或 AI 多模态分析使用。
