`simulator` 库提供基于真实浏览器的 Web 登录爆破能力，能处理动态登录页、JS 加密、验证码等复杂场景，按元素选择器定位用户名/密码/提交按钮并自动尝试字典。

典型使用场景：

- 登录爆破：`simulator.HttpBruteForce(targetUrl, opts...)` 对登录页爆破，配 `simulator.usernameSelector` / `simulator.passwordSelector` / `simulator.submitButtonSelector` 定位元素，`simulator.username` / `simulator.password`（或 `simulator.usernameList` / `simulator.passwordList`）提供字典。
- 验证码与判定：`simulator.captchaImgSelector` / `simulator.captchaInputSelector` / `simulator.captchaMode` 处理验证码，`simulator.successMatchers` / `simulator.loginDetectMode` 判定登录是否成功，`simulator.preAction` 在爆破前执行自定义 JS。

与相邻库的关系：`simulator` 走真实浏览器，专攻"前端加密 + 验证码"的登录爆破；与 `brute`（协议层爆破）、`rpa`/`crawlerx`（浏览器自动化）互补。
