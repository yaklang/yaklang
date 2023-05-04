// 创建一个 subfinder 扫描任务（小任务）
subfinder,err = tools.NewSubFinder()
if err != nil {
    die(err)
}

// 可以设置超时时间，超时会被 kill 掉
subfinder.SetTimeout("5m")

// 执行
results, err = subfinder.Exec("uestc.edu.cn")
if err != nil {
    die(err)
}

// 获取结果
dump(results)

/*
 ...
 (*subdomain.SubdomainResult)(0x1400015d0e0)({
  FromTarget: (string) (len=12) "uestc.edu.cn",
  FromDNSServer: (string) "",
  FromModeRaw: (int) 2,
  IP: (string) (len=13) "222.197.166.2",
  Domain: (string) (len=22) "www.cdyjy.uestc.edu.cn",
  Tags: ([]string) (len=1 cap=1) {
   (string) (len=11) "sitedossier"
  }
 }),
 ...
*/