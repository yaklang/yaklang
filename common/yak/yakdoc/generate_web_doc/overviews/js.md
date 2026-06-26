`js` 库内嵌一个 JavaScript 运行时（goja），可在 yak 脚本中执行 JS 代码、调用 JS 函数、解析 JS AST，并内置 CryptoJS、JSEncrypt、jsrsasign 等常用前端加密库，常用于还原前端加密逻辑、对抗 JS 混淆与参数加签。

典型使用场景：

- 执行与调用：`js.Run` 执行 JS 代码，`js.CallFunctionFromCode` 直接调用源码中的函数，`js.New` 创建可复用运行时，`js.ToValue` 在 yak/JS 值间转换。
- 解析：`js.Parse` / `js.ASTWalk` 解析与遍历 JS AST（用于分析前端逻辑）。
- 内置加密库：`js.libCryptoJSV3` / `js.libCryptoJSV4` / `js.libJSRSASign` / `js.libJsEncrypt` 注入常用前端加密库，`js.withVariable(s)` 注入变量。

与相邻库的关系：`js` 常配合 `crawler`/`crawlerx`（前端逻辑分析）、`codec`（加解密）、`fuzz`（构造加密参数）使用，用于"用前端的算法生成请求参数"的场景。
