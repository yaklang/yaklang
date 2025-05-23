package yakgrpc

import (
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func safeUTF8MITMV2Resp(response *ypb.MITMV2Response) *ypb.MITMV2Response {
	if response == nil {
		return nil
	}
	response.FilterData = safeMITMFilterData(response.FilterData)
	response.Replacers = safeMITMContentReplacerList(response.Replacers)
	response.Message = safeUTFExecResult(response.Message)
	response.Hooks = safeYakScriptHookList(response.Hooks)
	response.ManualHijackListAction = codec.StringUtf8SafeEscape(response.ManualHijackListAction)
	response.ManualHijackList = safeSingleManualHijackInfoList(response.ManualHijackList)
	return response
}

func safeSingleManualHijackInfoList(data []*ypb.SingleManualHijackInfoMessage) []*ypb.SingleManualHijackInfoMessage {
	if data == nil {
		return nil
	}
	for i := 0; i < len(data); i++ {
		data[i] = safeSingleManualHijackInfo(data[i])
	}
	return data
}

func safeSingleManualHijackInfo(data *ypb.SingleManualHijackInfoMessage) *ypb.SingleManualHijackInfoMessage {
	if data == nil {
		return nil
	}
	data.TaskID = codec.StringUtf8SafeEscape(data.TaskID)
	data.Status = codec.StringUtf8SafeEscape(data.Status)
	data.Tags = codec.StringArrayUtf8SafeEscape(data.Tags)
	data.URL = codec.StringUtf8SafeEscape(data.URL)
	data.RemoteAddr = codec.StringUtf8SafeEscape(data.RemoteAddr)
	data.WebsocketEncode = codec.StringArrayUtf8SafeEscape(data.WebsocketEncode)
	data.TraceInfo = safeTraceInfo(data.TraceInfo)
	data.Method = codec.StringUtf8SafeEscape(data.Method)
	return data
}

func safeUTF8MITMResp(response *ypb.MITMResponse) *ypb.MITMResponse {
	if response == nil {
		return nil
	}

	response.Url = codec.StringUtf8SafeEscape(response.Url)
	response.RemoteAddr = codec.StringUtf8SafeEscape(response.RemoteAddr)
	response.IncludeHostname = codec.StringArrayUtf8SafeEscape(response.IncludeHostname)
	response.ExcludeHostname = codec.StringArrayUtf8SafeEscape(response.ExcludeHostname)
	response.ExcludeSuffix = codec.StringArrayUtf8SafeEscape(response.ExcludeSuffix)
	response.IncludeSuffix = codec.StringArrayUtf8SafeEscape(response.IncludeSuffix)
	response.ExcludeMethod = codec.StringArrayUtf8SafeEscape(response.ExcludeMethod)
	response.ExcludeContentTypes = codec.StringArrayUtf8SafeEscape(response.ExcludeContentTypes)
	response.ExcludeUri = codec.StringArrayUtf8SafeEscape(response.ExcludeUri)
	response.IncludeUri = codec.StringArrayUtf8SafeEscape(response.IncludeUri)
	response.WebsocketEncode = codec.StringArrayUtf8SafeEscape(response.WebsocketEncode)
	response.Message = safeUTFExecResult(response.Message)
	response.FilterData = safeMITMFilterData(response.FilterData)
	response.Replacers = safeMITMContentReplacerList(response.Replacers)
	response.Hooks = safeYakScriptHookList(response.Hooks)
	response.TraceInfo = safeTraceInfo(response.TraceInfo)

	return response
}

func safeUTFExecResult(result *ypb.ExecResult) *ypb.ExecResult {
	if result == nil {
		return nil
	}

	result.Hash = codec.StringUtf8SafeEscape(result.Hash)
	result.OutputJson = codec.StringUtf8SafeEscape(result.OutputJson)
	result.RuntimeID = codec.StringUtf8SafeEscape(result.RuntimeID)
	return result
}

func safeMITMFilterData(data *ypb.MITMFilterData) *ypb.MITMFilterData {
	if data == nil {
		return nil
	}
	data.ExcludeSuffix = safeFilterDataItemList(data.ExcludeSuffix)
	data.IncludeSuffix = safeFilterDataItemList(data.IncludeSuffix)
	data.ExcludeUri = safeFilterDataItemList(data.ExcludeUri)
	data.IncludeUri = safeFilterDataItemList(data.IncludeUri)

	return data
}

func safeFilterDataItemList(data []*ypb.FilterDataItem) []*ypb.FilterDataItem {
	if data == nil {
		return nil
	}
	for i := 0; i < len(data); i++ {
		data[i] = safeFilterDataItem(data[i])
	}
	return data
}

func safeFilterDataItem(item *ypb.FilterDataItem) *ypb.FilterDataItem {
	if item == nil {
		return nil
	}
	item.Group = codec.StringArrayUtf8SafeEscape(item.Group)
	item.MatcherType = codec.StringUtf8SafeEscape(item.MatcherType)
	return item
}

func safeMITMContentReplacerList(data []*ypb.MITMContentReplacer) []*ypb.MITMContentReplacer {
	if data == nil {
		return nil
	}
	for i := 0; i < len(data); i++ {
		data[i] = safeMITMContentReplacer(data[i])
	}
	return data
}

func safeMITMContentReplacer(data *ypb.MITMContentReplacer) *ypb.MITMContentReplacer {
	if data == nil {
		return nil
	}
	data.Rule = codec.StringUtf8SafeEscape(data.Rule)
	data.Result = codec.StringUtf8SafeEscape(data.Result)
	data.Color = codec.StringUtf8SafeEscape(data.Color)
	data.ExtraTag = codec.StringArrayUtf8SafeEscape(data.ExtraTag)
	data.VerboseName = codec.StringUtf8SafeEscape(data.VerboseName)
	data.ExtraHeaders = safeHTTPHeaderList(data.ExtraHeaders)
	data.EffectiveURL = codec.StringUtf8SafeEscape(data.EffectiveURL)
	data.ExtraCookies = safeHTTPCookieSettingList(data.ExtraCookies)
	return data
}

func safeHTTPCookieSettingList(data []*ypb.HTTPCookieSetting) []*ypb.HTTPCookieSetting {
	if data == nil {
		return nil
	}
	for i := 0; i < len(data); i++ {
		data[i] = safeHTTPCookieSetting(data[i])
	}
	return data
}

func safeHTTPCookieSetting(data *ypb.HTTPCookieSetting) *ypb.HTTPCookieSetting {
	if data == nil {
		return nil
	}
	data.Key = codec.StringUtf8SafeEscape(data.Key)
	data.Value = codec.StringUtf8SafeEscape(data.Value)
	data.Path = codec.StringUtf8SafeEscape(data.Path)
	data.Domain = codec.StringUtf8SafeEscape(data.Domain)
	return data
}

func safeHTTPHeaderList(data []*ypb.HTTPHeader) []*ypb.HTTPHeader {
	if data == nil {
		return nil
	}
	for i := 0; i < len(data); i++ {
		data[i] = safeHTTPHeader(data[i])
	}
	return data
}

func safeHTTPHeader(data *ypb.HTTPHeader) *ypb.HTTPHeader {
	if data == nil {
		return nil
	}
	data.Header = codec.StringUtf8SafeEscape(data.Header)
	data.Value = codec.StringUtf8SafeEscape(data.Value)
	return data
}

func safeYakScriptHookList(data []*ypb.YakScriptHooks) []*ypb.YakScriptHooks {
	if data == nil {
		return nil
	}
	for i := 0; i < len(data); i++ {
		data[i] = safeYakScriptHook(data[i])
	}
	return data
}

func safeYakScriptHook(data *ypb.YakScriptHooks) *ypb.YakScriptHooks {
	if data == nil {
		return nil
	}
	data.HookName = codec.StringUtf8SafeEscape(data.HookName)
	data.Hooks = safeYakScriptHookItemList(data.Hooks)
	return data
}

func safeYakScriptHookItemList(item []*ypb.YakScriptHookItem) []*ypb.YakScriptHookItem {
	if item == nil {
		return nil
	}
	for i := 0; i < len(item); i++ {
		item[i] = safeYakScriptHookItem(item[i])
	}
	return item
}

func safeYakScriptHookItem(item *ypb.YakScriptHookItem) *ypb.YakScriptHookItem {
	if item == nil {
		return nil
	}
	item.YakScriptName = codec.StringUtf8SafeEscape(item.YakScriptName)
	item.Verbose = codec.StringUtf8SafeEscape(item.Verbose)
	return item
}

func safeTraceInfo(data *ypb.TraceInfo) *ypb.TraceInfo {
	if data == nil {
		return nil
	}
	data.AvailableDNSServers = codec.StringArrayUtf8SafeEscape(data.AvailableDNSServers)

	return data
}
