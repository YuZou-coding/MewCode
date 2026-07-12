# MewCode MCP 延迟发现任务拆解

## 1. 建立搜索工具单元测试

- 影响文件：`internal/external/tool_search_test.go`
- 依赖任务：无
- 参考资料定位：`ToolSearch.Execute`、`Manager.Client`
- 内容：覆盖关键词匹配、精确选择、无效精确名称、重复注册和失败隔离。

## 2. 完善搜索与精确选择

- 影响文件：`internal/external/tool_search.go`、`internal/external/manager.go`
- 依赖任务：1
- 参考资料定位：`candidateServers`、`matchesToolSearch`、`ServerNames`
- 内容：实现候选 server 选择、结构化结果、缓存复用和无效精确名称短路。

## 3. 注册内置搜索工具

- 影响文件：`internal/app/app.go`、`internal/app/app_test.go`
- 依赖任务：2
- 参考资料定位：`App.Run`、工具注册初始化
- 内容：启动时只注册搜索工具，验证不会初始化 MCP server。

## 4. 接入 Agent 动态工具列表

- 影响文件：`internal/agent/agent.go`、`internal/agent/agent_test.go`
- 依赖任务：3
- 参考资料定位：`Agent.toolsForTurn`、Skill 工具过滤
- 内容：保证启用 SkillManager 时，发现后的工具仍在下一轮可见。

## 5. 验证权限与结果回灌

- 影响文件：`internal/agent/agent_test.go`、`internal/e2e/external_tools_e2e_test.go`
- 依赖任务：3、4
- 参考资料定位：`executeToolBatch`、外部工具错误回灌测试
- 内容：验证搜索和远端调用经过既有权限与工具结果流程。

## 6. 更新提示词与使用文档

- 影响文件：`internal/prompt/global.go`、`README.md`
- 依赖任务：2、3
- 参考资料定位：工具使用规则、外部工具服务器章节
- 内容：说明按需搜索、精确选择、缓存和失败行为。

## 7. 接入 TUI 状态

- 影响文件：`internal/tui/loop.go`、`internal/tuiapp/layout.go`
- 依赖任务：2、3
- 参考资料定位：`Controller.Status`、状态栏渲染
- 内容：显示当前已成功连接的 MCP server 数量。

## 8. 接入主流程

- 影响文件：`internal/app/app.go`、`internal/agent/agent.go`、`internal/external/tool_search.go`
- 依赖任务：1-7
- 参考资料定位：Registry 初始化、Agent 每轮请求、ToolSearch 执行
- 内容：串联配置加载、搜索发现、动态注册和后续远端调用。

## 9. 端到端验证

- 影响文件：`internal/e2e/external_tools_e2e_test.go`
- 依赖任务：8
- 参考资料定位：现有 stdio、HTTP MCP 测试
- 内容：验证启动零连接、搜索、下一轮调用、连接复用、失败隔离与全量测试。
