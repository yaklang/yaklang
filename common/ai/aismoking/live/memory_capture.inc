// Decode the full response body supplied by ai.finishReasonCallback. The HTTP
// diagnostic callback only keeps a 4 KiB preview, which may omit a late call.
func memoryDecodeResponse(raw) {
    decoded = {"content":"","reasoning_content":"","tool_calls":[],"finish_reason":"","parse_errors":[]}
    // Raw mirrors retain HTTP chunk framing, possibly inside a JSON key/value.
    // Keep the original body in the caller's sample; decode only for inspection.
    firstLine = str.SplitN(raw,"\n",2)[0]
    if re.Match(`^[0-9a-fA-F]+(;[^\r\n]*)?\r?$`,firstLine) {
        unchunked,chunkError = codec.DecodeChunked([]byte(raw))
        raw = string(unchunked)
        if chunkError != nil { decoded.parse_errors = append(decoded.parse_errors,"HTTP chunk decode: "+string(chunkError)) }
    }
    callsByIndex = {}
    frames = []
    trimmed = str.TrimSpace(raw)
    if str.HasPrefix(trimmed,"{") {
        frame,err = json.New(trimmed)
        if err != nil { decoded.parse_errors = append(decoded.parse_errors,string(err)) } else { frames = append(frames,frame.Value()) }
    } else {
        for line in str.Split(raw,"\n") {
            line = str.TrimSpace(line)
            if !str.HasPrefix(line,"data:") { continue }
            payload = str.TrimSpace(line[5:])
            if payload == "" || payload == "[DONE]" { continue }
            frame,err = json.New(payload)
            if err != nil { decoded.parse_errors = append(decoded.parse_errors,string(err)) } else { frames = append(frames,frame.Value()) }
        }
    }
    for frame in frames {
        if !("choices" in frame) { continue }
        for choice in frame.choices {
            if "finish_reason" in choice && choice.finish_reason != nil { decoded.finish_reason = choice.finish_reason }
            message = {}
            if "delta" in choice { message = choice.delta } else if "message" in choice { message = choice.message }
            if "content" in message && message.content != nil { decoded.content += string(message.content) }
            if "reasoning_content" in message && message.reasoning_content != nil { decoded.reasoning_content += string(message.reasoning_content) }
            if !("tool_calls" in message) { continue }
            for delta in message.tool_calls {
                index = 0
                if "index" in delta { index = delta.index }
                choiceIndex = 0
                if "index" in choice { choiceIndex = choice.index }
                key = sprintf("%v/%v",choiceIndex,index)
                if !(key in callsByIndex) {
                    call = {"choice_index":choiceIndex,"index":index,"id":"","name":"","arguments":""}
                    callsByIndex[key] = call
                    decoded.tool_calls = append(decoded.tool_calls,call)
                }
                call = callsByIndex[key]
                if "id" in delta { call.id = delta.id }
                if !("function" in delta) { continue }
                functionDelta = delta["function"]
                if "name" in functionDelta {
                    part = functionDelta.name
                    if str.HasPrefix(part,call.name) { call.name = part } else { call.name += part }
                }
                if "arguments" in functionDelta { call.arguments += functionDelta.arguments }
            }
        }
    }
    return decoded
}
