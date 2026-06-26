`httpserver` 库用于在脚本里快速起一个 HTTP/HTTPS 服务端，支持自定义路由、本地文件服务、WebSocket、TLS 与验证码路由，常用于接收回连、托管 PoC 页面、搭建临时测试服务。

典型使用场景：

- 启动服务：`httpserver.Serve(host, port, opts...)` 起服务，`httpserver.LocalFileSystemServe` 直接对外提供本地目录。
- 路由处理：`httpserver.routeHandler` 注册路径处理器，`httpserver.handler` 注册全局处理器，`httpserver.wsRouteHandler` 处理 WebSocket，`httpserver.captchaRouteHandler` 提供验证码路由，`httpserver.localFileSystemHandler` 提供静态目录。
- 传输与上下文：`httpserver.tlsCertAndKey` 配置 HTTPS 证书，`httpserver.context` 控制生命周期。

与相邻库的关系：`httpserver` 是服务端能力，常与 `facades`（多协议恶意服务）、`dnslog`（带外）、`csrf`（托管 PoC）配合，用于接收交互/回连的场景。
