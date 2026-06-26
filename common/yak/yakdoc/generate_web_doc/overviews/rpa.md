`rpa` 库提供基于浏览器的流程自动化（RPA）能力，可模拟点击、输入、选择、登录爆破等真实用户操作，常用于复杂登录流程的自动化、带验证码的爆破与动态站点交互。

典型使用场景：

- 自动化遍历：`rpa.Start(url, opts...)` 启动浏览器自动化并返回请求流，配 `rpa.click` / `rpa.input` / `rpa.select` 模拟交互，`rpa.depth` / `rpa.maxUrl` / `rpa.whiteDomain` / `rpa.blackDomain` 控制范围。
- 登录爆破：`rpa.Bruteforce(url, opts...)` 对登录页爆破，配 `rpa.bruteUserElement` / `rpa.brutePassElement` / `rpa.bruteButtonElement` 定位元素，`rpa.bruteUsername` / `rpa.brutePassword` 提供字典，`rpa.bruteCaptchaElement` 处理验证码。

与相邻库的关系：`rpa` 走真实浏览器，与 `crawlerx`（浏览器爬虫）、`simulator`（模拟登录爆破）、`browser`（浏览器实例）思路相通，专攻"需要真实交互"的自动化场景。
