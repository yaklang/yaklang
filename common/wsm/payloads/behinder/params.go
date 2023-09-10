package behinder

type ParamItem struct {
	Key   string
	Value string
}

type Params struct {
	ParamItem []*ParamItem
}

type ExecParamsConfig func(p *Params)
type FileParamsConfig func(p *Params)

func SetCommandPath(path string) ExecParamsConfig {
	return func(p *Params) {
		p.ParamItem = append(p.ParamItem, &ParamItem{
			Key:   "path",
			Value: path,
		})
	}
}

// SetNotEncrypt WebShell 返回的结果是否加密
func SetNotEncrypt() ExecParamsConfig {
	return func(p *Params) {
		p.ParamItem = append(p.ParamItem, &ParamItem{
			Key:   "notEncrypt",
			Value: "false",
		})
	}
}

// SetPrintMode https://www.cnblogs.com/qingmuchuanqi48/p/12079415.html
func SetPrintMode() ExecParamsConfig {
	return func(p *Params) {
		p.ParamItem = append(p.ParamItem, &ParamItem{
			Key:   "forcePrint",
			Value: "false",
		})
	}
}

func ProcessParams(params map[string]string, opts ...ExecParamsConfig) map[string]string {
	paramsEx := &Params{}
	for _, opt := range opts {
		opt(paramsEx)
	}

	for _, item := range paramsEx.ParamItem {
		if _, ok := params[item.Key]; ok {
			params[item.Key] = item.Value
		}
		if item.Key == "notEncrypt" {
			params["notEncrypt"] = item.Value
		}
		if item.Key == "forcePrint" {
			params["forcePrint"] = item.Value
		}
	}

	return params
}

func SetFileMode(mode string) FileParamsConfig {
	return func(p *Params) {
		p.ParamItem = append(p.ParamItem, &ParamItem{
			Key:   "mode",
			Value: mode,
		})
	}
}
