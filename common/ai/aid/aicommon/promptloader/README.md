# AI prompt resources

All fixed AI prompt text and its associated response schemas live below `prompts/` and are read through this package. Paths passed to `MustLoad` and `ReadFile` are relative to that directory. `mainloop/` contains shared main-loop templates; only sections that differ by protocol have `textstream/` and `functioncall/` variants. `ai/`, `aiforge/`, and `inline/` retain the originating package in their paths to avoid name collisions.

Edit the source files directly. Ordinary builds embed them without a generation step. Release builds use `gzip_embed`; `scripts/generate-compressed-assets.sh` regenerates the single `prompts.tar.gz` archive from the same source tree before compiling. `TestCompressedPromptResources` compares every loaded resource with its source in both build modes and fails if the archive is stale.
