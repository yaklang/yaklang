`imageutils` 库提供图像提取能力，从各种输入（文件、字节、文档、PDF 等）中抽取图片，常用于多模态 AI 预处理、文档取证与 OCR 前置处理。

典型使用场景：

- 提取图片：`imageutils.ExtractImage(i)` 从任意输入抽取图片流，`imageutils.ExtractImageFromFile` 从文件抽取（可配 `imageutils.context` 控制取消）。

与相邻库的关系：`imageutils` 抽出的图片常交给 AI 多模态（`ai` 的 VisionChat）、`ffmpeg`（视频帧）、`fileparser`（文件解析）等后续处理。
