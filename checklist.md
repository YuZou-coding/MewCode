# MewCode MCP 状态栏计数验收清单

- [x] 配置 5 个 MCP server、尚未初始化任何 server 时，状态栏显示 `mcp 0/5`。
- [x] 5 个配置中已有 2 个 server 初始化并缓存时，状态栏显示 `mcp 2/5`。
- [x] 没有 MCP 配置时，状态栏显示 `mcp 0/0`。
- [x] 读取配置总数不会创建 MCP client、发送 initialize 或调用 tools/list。
- [x] 状态栏仍只占一行，并沿用宽度小于 60 列时隐藏 MCP 信息的响应式规则。
- [x] E2E：从 controller 状态到 TUI 渲染可观察到 `mcp 已连接/已配置`，全量 `go test -count=1 ./...` 通过。
