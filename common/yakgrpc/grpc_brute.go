package yakgrpc

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"strings"
)

const startBruteScript = `yakit.AutoInitYakit()

debug = false

yakit.Info("开始检查执行参数")

targetFile := cli.String("target-file", cli.setRequired(true))
userList := cli.String("user-list-file")
passList := cli.String("pass-list-file")
concurrent := cli.Int("concurrent")
taskConcurrent := cli.Int("task-concurrent")
minDelay, maxDelay := cli.Int("delay-min", cli.setDefault(3)), cli.Int("delay-max", cli.setDefault(5))
pluginFile := cli.String("plugin-file")
okToStop := cli.Bool("ok-to-stop")
replaceDefaultUsernameDict := cli.Bool("replace-default-username-dict")
replaceDefaultPasswordDict := cli.Bool("replace-default-password-dict")
finishingThreshold = cli.Int("finishing-threshold", cli.setDefault(1))

yakit.Info("检查爆破类型")
bruteTypes = cli.String("types")
if bruteTypes == "" {
    yakit.Error("没有指定爆破类型")
    if !debug {
        die("exit normal")
    }
    bruteTypes = "ssh"
}

// TargetsConcurrent
// TargetTaskConcurrent
// DelayerMin DelayerMax
// BruteCallback
// OkToStop
// FinishingThreshold
// OnlyNeedPassword
wg := sync.NewWaitGroup()
defer wg.Wait()

yakit.Info("扫描目标预处理")
// 处理扫描目标
raw, _ := file.ReadFile(targetFile)
if len(raw) == 0 {
    yakit.Error("BUG：读取目标文件失败！")
    if !debug {
        return
    }
    raw = []byte("127.0.0.1:23")
}
target = str.ParseStringToLines(string(raw))

targetRaw = make([]string)
for _, t := range target {
    host, port, err := str.ParseStringToHostPort(t)
    if err != nil {
        targetRaw = append(targetRaw, t)
    }else{
        targetRaw = append(targetRaw, str.HostPort(host, port))
    }
}
target = targetRaw

yakit.Info("用户自定义字典预处理")
// 定义存储用户名与密码的字典
userdefinedUsernameList = make([]string)
userdefinedPasswordList = make([]string)

// 获取用户列表
userRaw, _ := file.ReadFile(userList)
if len(userRaw) <= 0 {
    yakit.Error("用户文件字典获取失败")
}else{
    userdefinedUsernameList = str.ParseStringToLines(string(userRaw))
}

// 获取用户密码
passRaw, _ := file.ReadFile(passList)
if len(passRaw) <= 0 {
    yakit.Error("用户密码文件获取失败")
}else{
    userdefinedPasswordList = str.ParseStringToLines(string(passRaw))
}

opt = []

if minDelay > 0 && maxDelay > 0 {
    yakit.Info("单目标测试随机延迟：%v-%v/s", minDelay, maxDelay)
    opt = append(opt, brute.minDelay(minDelay), brute.maxDelay(maxDelay))
}

if finishingThreshold > 0 {
    opt = append(opt, brute.finishingThreshold(finishingThreshold))
}

if concurrent > 0 {
    yakit.Info("设置最多同时爆破目标：%v", concurrent)
    opt = append(opt, brute.concurrentTarget(concurrent))
}

if taskConcurrent > 0 {
    yakit.Info("设置单目标爆破并发：%v", taskConcurrent)
    opt = append(opt, brute.concurrent(taskConcurrent))
}


tableName = "可用爆破结果表"
columnType = "TYPE"
columnTarget = "TARGET"
columnUsername = "USERNAME"
columnPassword = "PASSWORD"
yakit.EnableTable(tableName, [columnType, columnTarget, columnUsername, columnPassword])

scan = func(bruteType) {
    yakit.Info("启用针对 %v 的爆破程序", bruteType)
    wg.Add(1)
    go func{
        defer wg.Done()

        tryCount = 0
        success = 0
        failed = 0
        finished = 0

        uL = make([]string)
        pL = make([]string)
        if (!replaceDefaultUsernameDict) {
            uL = append(uL, brute.GetUsernameListFromBruteType(bruteType)...)
        }

        if (!replaceDefaultPasswordDict) {
			pL = append(pL, brute.GetPasswordListFromBruteType(bruteType)...)
        }

        instance, err := brute.New(
            string(bruteType),
            brute.userList(append(userdefinedUsernameList, uL...)...),
            brute.passList(append(userdefinedPasswordList, pL...)...),
            brute.debug(true),
            brute.okToStop(okToStop),
            opt...
        )
        if err != nil {
            yakit.Error("构建弱口令与未授权扫描失败：%v", err)
            return
        }

        res, err := instance.Start(target...)
        if err != nil {
            yakit.Error("输入目标失败：%v", err)
            return
        }

        for result := range res {
            tryCount++
            yakit.StatusCard("总尝试次数: "+bruteType, tryCount, bruteType, "total")
            result.Show()

            if result.Ok {
                success++
                yakit.StatusCard("成功次数: "+bruteType, success, bruteType, "success")
                risk.NewRisk(
                    riskTarget, risk.severity("high"), risk.type("weak-pass"),
                    risk.typeVerbose("弱口令"),
                    risk.title(sprintf("Weak Password[%v]：%v user(%v) pass(%v)", result.Type, result.Target, result.Username, result.Password)),
                    risk.titleVerbose(sprintf("弱口令[%v]：%v user(%v) pass(%v)", result.Type, result.Target, result.Username, result.Password)),
                    risk.details({"username": result.Username, "password": result.Password, "target": result.Target}),
                )
                yakit.Output(yakit.TableData(tableName, {
                    columnType: result.Type,
                    columnTarget: result.Target,
                    columnUsername: result.Username,
                    columnPassword: result.Password,
                    "id": tryCount,
                    "bruteType": bruteType,
                }))
            } else {
                failed++
                yakit.StatusCard("失败次数: " + bruteType, failed, bruteType, "failed")
            }
        }
    }
}

for _, t := range str.Split(bruteTypes, ",") {
    scan(t)
}`

func (s *Server) StartBrute(params *ypb.StartBruteParams, stream ypb.Yak_StartBruteServer) error {
	reqParams := &ypb.ExecRequest{Script: startBruteScript}

	types := utils.PrettifyListFromStringSplited(params.GetType(), ",")
	for _, t := range types {
		h, err := bruteutils.GetBruteFuncByType(t)
		if err != nil || h == nil {
			return utils.Errorf("brute type: %v is not available", t)
		}
	}
	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "types", Value: params.GetType()})

	targetFile, err := utils.DumpHostFileWithTextAndFiles(params.Targets, "\n", params.TargetFile)
	if err != nil {
		return err
	}
	defer os.RemoveAll(targetFile)
	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "target-file", Value: targetFile})

	// 解析用户名
	userListFile, err := utils.DumpFileWithTextAndFiles(
		strings.Join(params.Usernames, "\n"), "\n", params.UsernameFile,
	)
	if err != nil {
		return err
	}
	defer os.RemoveAll(userListFile)
	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "user-list-file", Value: userListFile})

	// 是否使用默认字典？
	if params.GetReplaceDefaultPasswordDict() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "replace-default-password-dict"})
	}

	if params.GetReplaceDefaultUsernameDict() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "replace-default-username-dict"})
	}

	// 解析密码
	passListFile, err := utils.DumpFileWithTextAndFiles(
		strings.Join(params.Passwords, "\n"), "\n", params.PasswordFile,
	)
	if err != nil {
		return err
	}
	defer os.RemoveAll(passListFile)
	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "pass-list-file", Value: passListFile})

	// ok to stop
	if params.GetOkToStop() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "ok-to-stop", Value: ""})
	}

	if params.GetConcurrent() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "concurrent", Value: fmt.Sprint(params.GetConcurrent())})
	}

	if params.GetTargetTaskConcurrent() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "task-concurrent", Value: fmt.Sprint(params.GetTargetTaskConcurrent())})
	}

	if params.GetDelayMin() > 0 && params.GetDelayMax() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "delay-min", Value: fmt.Sprint(params.GetDelayMin())})
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "delay-max", Value: fmt.Sprint(params.GetDelayMax())})
	}

	return s.Exec(reqParams, stream)
}

func (s *Server) GetAvailableBruteTypes(ctx context.Context, req *ypb.Empty) (*ypb.GetAvailableBruteTypesResponse, error) {
	return &ypb.GetAvailableBruteTypesResponse{Types: bruteutils.GetBuildinAvailableBruteType()}, nil
}
