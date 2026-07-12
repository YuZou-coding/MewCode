# MewCode MCP 状态栏计数任务拆解

## 1. 增加 MCP 配置计数

- 影响文件：`internal/external/manager.go`、`internal/external/manager_test.go`
- 依赖任务：无
- 参考资料定位：`Manager.CachedCount`、`Manager.ServerNames`
- 内容：为 manager 增加只读取已加载配置数量的接口，证明不会创建 client 或触发连接。

## 2. 扩展通用 UI 状态

- 影响文件：`internal/command/types.go`、`internal/tui/loop.go`、`internal/tui/loop_test.go`
- 依赖任务：1
- 参考资料定位：`command.State`、`Controller.Status`
- 内容：分别传递 MCP 已连接数量和已配置数量，保持 controller 为 UI 与 manager 的边界。

## 3. 渲染状态栏比例

- 影响文件：`internal/tuiapp/layout.go`、`internal/tuiapp/model_test.go`
- 依赖任务：2
- 参考资料定位：`Model.statusLine`、`TestStatusLineShowsConnectedMCPCount`
- 内容：把现有 MCP 状态改为已连接/已配置格式，并覆盖有配置和无配置场景。

## 4. 接入主流程

- 影响文件：`internal/tui/loop.go`、`internal/tuiapp/layout.go`
- 依赖任务：1、2、3
- 参考资料定位：`Controller.Status`、`ExternalManager.CachedCount`
- 内容：确认每次状态刷新读取最新缓存数量和稳定配置总数，不触发 MCP 初始化。

## 5. 端到端验证

- 影响文件：`internal/tuiapp/model_test.go`、`checklist.md`
- 依赖任务：全部任务
- 参考资料定位：状态栏渲染测试、manager lazy discovery 测试
- 内容：验证零连接/多配置、连接数增长和零配置输出，运行全量 Go 测试并重新安装二进制。
