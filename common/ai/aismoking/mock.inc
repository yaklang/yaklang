// Support library; runnable cases live beside this file. Includes are relative to repository root.
smokeCtx, smokeCancel = context.WithCancel(context.WithTimeoutSeconds(240))
defer smokeCancel()
smokeRoot = os.Getenv("AISMOKING_OUTPUT")
if smokeRoot == "" { smokeRoot = file.Join(os.TempDir(), sprintf("ai-smoking-%d", time.Now().UnixNano())) }
file.MkdirAll(smokeRoot)~
smokeMutex = sync.NewMutex()
smokeRequests, smokeErrors, smokeHandler = [], [], nil
smokePort = os.GetRandomAvailableTCPPort()
smokeBase = "http://" + str.HostPort("127.0.0.1", smokePort)

func smokeFrame(w, delta, finish) {
    w.Write("data: " + string(json.Marshal({"choices":[{"index":0,"delta":delta,"finish_reason":finish}]})~) + "\n\n")~
    w.Flush()
}
func smokeAnswer(w, native, name, args) {
    args = json.loads(string(json.Marshal(args)~))
    w.Header().Set("Content-Type", "text/event-stream")
    raw = json.dumps(args)
    if native {
        half = len(raw) / 2
        smokeFrame(w, {"tool_calls":[{"index":0,"id":"smoke-output","type":"function","function":{"name":name,"arguments":raw[:half]}}]}, nil)
        smokeFrame(w, {"tool_calls":[{"index":0,"function":{"arguments":raw[half:]}}]}, nil)
        smokeFrame(w, {}, "tool_calls")
    } else {
        args["@action"], args["identifier"] = name, name
        smokeFrame(w, {"content":json.dumps(args)}, "stop")
    }
    w.Write("data: [DONE]\n\n")~
}
func smokePrompt(body) {
    prompt = ""
    for message in body.messages { prompt += string(message.content) + "\n" }
    return prompt
}
func smokeApprove(op, payload, response) {
    assert payload.id != "", "review endpoint ID is required"
    op.SendInputEvent(aim.NewInputEvent({"IsInteractiveMessage":true,"InteractiveId":payload.id,"InteractiveJSONInput":json.dumps(response)})~)~
}
// Real gateway projection/HTTP/SSE. Only provider answers are mocked.
smokeModel = func(config, request) {
    text, err = ai.Chat("SMOKE_CALLER:" + request.GetCallerLabel() + "\n" + request.GetPrompt(),
        ai.type("openai"), ai.baseURL(smokeBase + "/v1/chat/completions"), ai.apiKey("local-fixture"),
        ai.model("ai-smoking"), ai.noHttps(true), func(c) { c.DisableProviderFallback = true; c.Context = smokeCtx; c.FunctionCallRetryTimes = 1 }, request.GetExtraSpecOpts()...)
    if err != nil { return nil, err }
    response = config.NewAIResponse()
    response.EmitOutputStream(str.NewReader(text))
    response.Close()
    return response, nil
}
go httpserver.Serve("127.0.0.1", smokePort, httpserver.context(smokeCtx), httpserver.handler(func(w, req) {
    if req.URL.Path == "/ready" { w.Write("ready"); return }
    try {
        body = json.loads(string(io.ReadAll(req.Body)~))
        smokeMutex.Lock()
        smokeRequests = append(smokeRequests, body)
        seq = len(smokeRequests)
        smokeMutex.Unlock()
        assert seq < 200, "mock provider did not converge"
        // Credentials/headers are never included in samples.
        file.SaveJson(file.Join(smokeRoot, sprintf("request-%03d.json", seq)), body)~
        smokeHandler(w, body)
    } catch err {
        println("MOCK FAILURE: ",err)
        smokeMutex.Lock()
        smokeErrors = append(smokeErrors, string(err))
        smokeMutex.Unlock()
        smokeCancel()
        w.WriteHeader(500)
        w.Write("AI smoke fixture failed")
    }
}))
smokeReady = false
for i = 0; i < 100; i++ {
    _, err = http.Get(smokeBase + "/ready")
    if err == nil { smokeReady = true; break }
    time.Sleep(0.02)
}
assert smokeReady, "mock provider did not start"

func smokeCheck() {
    assert len(smokeErrors) == 0, json.dumps(smokeErrors)
    file.SaveJson(file.Join(smokeRoot, "summary.json"), {"requests":len(smokeRequests),"errors":smokeErrors,"provider_cache_hit_rate":nil})~
    printf("PASS: %d provider requests; samples: %s\n", len(smokeRequests), smokeRoot)
    println("AISMOKING_PASS")
}
