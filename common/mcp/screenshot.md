# Yakit screenshot

The optional `screenshot` tool set provides the `screenshot` MCP tool. Small
images are returned as an MCP `image` content block containing a Base64 PNG,
plus a text block with the dimensions and capture time. Larger images are
automatically saved as PNG files, and the tool returns only file metadata.

Enable the `screenshot` tool set in the MCP server configuration (`Tool:
["screenshot"]` for `StartMcpServer`, or include `screenshot` in the CLI `-t`
selection). `--enable-all` also includes it. Restart the updated Yak engine and
Yakit Electron main process, and connect Yakit to the engine hosting MCP.

Example `tools/call` parameters:

```json
{"name":"screenshot","arguments":{}}
```

## Saving and response size

| Argument | Default | Behavior |
| --- | --- | --- |
| `savePath` | Omitted | A PNG file path on the MCP engine machine. Specifying it always saves the image and returns file metadata only. Relative paths resolve under `YAKIT_HOME/screenshots`; absolute paths are also accepted. |
| `maxInlineBytes` | `1048576` | Maximum **Base64-encoded image bytes** to include in the MCP response, between 0 and 1048576. Set `0` to always save a file. |

For example, a 4 MiB PNG becomes roughly 5.33 MiB after Base64 encoding. With
the default 1 MiB inline limit it is saved to disk, and none of that image data
is included in the MCP tool response. The original PNG is preserved without
resizing or lossy compression. Images exactly at the inline limit stay inline.

Automatic saves use:

```text
<YAKIT_HOME>/screenshots/YYYY-MM-DD/screenshot-HHMMSS-<uuid>.png
```

`YAKIT_HOME` normally resolves to `~/yakit-projects`. Date folders and filenames
use the capture's local date/time. UUIDs avoid collisions between captures.
These are persistent evidence files, not files under the temporary directory;
they are not automatically removed. Parent directories are created as needed.
Existing files are never overwritten, and failed writes return errors instead
of falling back to a large inline response.

Save with a meaningful relative name:

```json
{"name":"screenshot","arguments":{"savePath":"idor/request-response.png"}}
```

Always save, generating a filename automatically:

```json
{"name":"screenshot","arguments":{"maxInlineBytes":0}}
```

A saved result contains one text content block with JSON such as:

```json
{
  "delivery": "file",
  "path": "/home/user/yakit-projects/screenshots/idor/request-response.png",
  "storageLocation": "mcp_engine",
  "mimeType": "image/png",
  "width": 1920,
  "height": 1080,
  "capturedAt": "2026-09-08 12:34:56 +08:00",
  "sizeBytes": 4194304,
  "base64Bytes": 5592408,
  "maxInlineBytes": 1048576,
  "reason": "savePath"
}
```

`reason` is `savePath`, `file_requested`, or `inline_limit_exceeded`. A returned
path belongs to the **MCP engine machine**, which can differ from the Yakit
desktop or MCP caller. It is not a download URL; remote callers need file access
to that machine to retrieve the saved image.

The inline limit applies to the **MCP response**. Desktop-to-engine transfer
still uses the existing Base64 duplex protocol and its 30 MiB encoded-image
limit (the engine accepts at most 32 MiB for the enclosing JSON message).

## Capture behavior

The capture covers the main Yakit window's current visible page, including open
dialogs rendered inside that page. It does not scroll, switch tabs, capture OS
dialogs. Electron captures the page directly; no desktop screen
recording permission is needed. The PNG retains the native capture resolution.

A white-on-dark watermark is drawn at the bottom left on a detached canvas.
It uses the **Yakit desktop computer's local system time**, even when the engine
runs remotely. Format: `2026-09-08 12:34:56 +08:00`. The actual page DOM is unchanged.

The existing `DuplexConnection` stream carries capability subscription, request
IDs and replies; no additional port or protobuf RPC is introduced. Exactly one
capable frontend must be connected. Missing/older frontends, multiple frontends,
closed windows and capture failures return explicit errors. Desktop capture is
bounded to 10 seconds; the engine waits at most 15 seconds (or the caller's
shorter deadline). Replies are matched to both request ID and selected stream.
