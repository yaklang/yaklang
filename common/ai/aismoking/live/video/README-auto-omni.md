# Automatic Omni knowledge extraction

`rag.BuildCollectionFromFile`, `rag.BuildIndexKnowledgeFromFile`, and the built-in
`构建知识库` plugin probe the locally configured Omni model before processing a
video, image, or PDF. Text files continue through text chunking.

The default AIBalance model is `qwen3.8-omni-flash`. A configured Omni model's
credentials, endpoint, headers and proxy are preserved. Credentials are supplied
through local provider configuration or `rag.omniConfig`; they are never stored
in this example.

```yak
opts = [
    rag.omniMode("auto"),
    rag.disableERM(true),
    liteforge.omniSegmentSeconds(20),
    liteforge.omniMaxBase64Bytes(7 * 1024 * 1024),
    rag.onError(func(err) { println("Build failed:", err) }),
]
entries = rag.BuildCollectionFromFile("training-media", "/path/lecture.mp4", opts...)~
for entry in entries {
    println(entry.KnowledgeTitle)
}
```

`omniMode("auto")` checks image input support once per input file. An unavailable
model uses the original extraction path. A failed media request falls back to
the original vision/video extractor for that page or clip. `required` reports
failures without fallback; `disabled` selects the original path immediately.
`rag.onError` receives asynchronous failures; drain the result channel before
checking errors, as demonstrated by `build-knowledge-auto-omni.yak`.

For an explicit provider configuration:

```yak
omni = rag.omniConfig(
    ai.type("aibalance"),
    ai.model("qwen3.8-omni-flash"),
    ai.apiKey(os.Getenv("AIBALANCE_API_KEY")),
)
info = liteforge.CheckOmniAvailable(omni)~
println(info.Provider, info.Model)
```

Video is reencoded to MP4 with audio, at most 720 pixels high and 2 frames per
second. The default segment duration is 20 seconds. The 7 MiB limit covers the
complete base64 data URI; oversized clips are subdivided before encoding and
upload. Temporary clips are removed after processing. Images and PDF pages are
resized to at most 2048 pixels on their longest edge, with further reductions
when necessary to fit the payload limit. Extracted content keeps its input path,
PDF page number, or original video time range.

Video processing needs ffmpeg; PDF rendering needs page2img. Install them with
the existing toolbox, then start the build:

```yak
toolbox.Install("ffmpeg")~
toolbox.Install("page2image")~
```

Run a live build with a media fixture containing a known fact:

```sh
yak common/ai/aismoking/live/video/build-knowledge-auto-omni.yak \
  --files /path/recovery.mp4 --kb-name recovery-training \
  --segment-seconds 3 --verify ORBIT-731
```

This calls the real model and embedding service, waits for successful index and
vector writes, then verifies the media's known fact in the persisted entries.
`--omni-mode auto` also enables availability fallback. The core plugin exposes
`--omni-mode`, `--omni-model`, `--omni-segment-seconds`, and `--omni-payload-bytes`
through the existing gRPC plugin parameter flow.

The skill-guided `manage_knowledge_base` tool accepts `omni_mode`, `omni_model`,
`omni_segment_seconds`, and `omni_payload_bytes` inside `build_options`. Direct
Yak calls use `aiknowledge.Build` / `BuildFromFile` with variadic options. For
example, after creating a knowledge base:

```yak
aiknowledge.BuildFromFile("training-media", "/path/lecture.mp4",
    aiknowledge.buildMode("chunks"), aiknowledge.omniMode("required"),
    aiknowledge.omniSegmentSeconds(20))~
```

`manage-media-knowledge-omni.yak` creates a new knowledge base through that API,
builds its media content, then verifies both keyword and semantic retrieval with
an explicit knowledge base name and token budget.
