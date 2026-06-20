`whisper` 库提供语音转写能力（基于 Whisper），把音频转换为 SRT 字幕文件并管理字幕，常用于音视频内容分析、取证转写与 AI 多模态预处理。

典型使用场景：

- 转写：`whisper.ConvertAudioToSRTFile(audio)` 把音频转为 SRT 字幕文件。
- 字幕管理：`whisper.CreateSRTManager(srtPath)` 创建字幕管理器，对字幕做读取与处理。

与相邻库的关系：`whisper` 常承接 `ffmpeg`（从视频抽取音频）的产出，转写后的文本可交给 `ai`/`rag` 做内容分析与知识入库。
