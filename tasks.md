# MewCode 通用 MCP 工具路由任务拆解

## 1. 增加 MCP server 元数据配置解析

- 影响文件：`internal/external/config.go`、`internal/external/config_test.go`
- 依赖任务：无
- 参考资料定位：`ServerConfig`、`assignConfigField`、`parseInlineList`、`TestLoadServersFile`
- 内容：给 `ServerConfig` 增加 `Description string`、`Capabilities []string`、`Keywords []string`、`Examples []string`，解析同名 YAML 字段并保持旧配置兼容。

## 2. 为 tool_search 建立匹配解释模型

- 影响文件：`internal/external/tool_search.go`、`internal/external/tool_search_test.go`
- 依赖任务：1
- 参考资料定位：`ToolSearch.Execute`、`candidateServers`、`matchesToolSearch`
- 内容：引入内部匹配结果结构，统一记录 server、local tool name、description、capabilities、matched 字段、score、recommended。

## 3. 使用 server 元数据粗筛候选 server

- 影响文件：`internal/external/tool_search.go`、`internal/external/tool_search_test.go`
- 依赖任务：1、2
- 参考资料定位：`candidateServers`、`Manager.ServerNames`
- 内容：让查询命中 server 描述、能力、关键词或示例时，只连接相关 server；无元数据命中时保持现有回退扫描行为。

## 4. 使用远端工具描述细筛并排序

- 影响文件：`internal/external/tool_search.go`、`internal/external/tool_search_test.go`
- 依赖任务：2、3
- 参考资料定位：`matchesToolSearch`、`uniqueToolName`、`RemoteTool`
- 内容：综合 server 元数据和远端工具名称/描述计算分数，按分数降序返回；最高分候选标记为 recommended。

## 5. 强化稳定系统提示的 MCP 路由规则

- 影响文件：`internal/prompt/global.go`、`internal/prompt/module_test.go`
- 依赖任务：无
- 参考资料定位：`DefaultModules` 的 `tool` 模块、`TestDefaultModulesContainExpectedResponsibilities`
- 内容：补充通用 MCP 判断规则，覆盖最新/官方文档、外部 SaaS/API、浏览器实时状态、数据库、Issue 系统、云服务、项目外知识库等场景。

## 6. 更新用户文档和配置示例

- 影响文件：`README.md`
- 依赖任务：1、2、3、4、5
- 参考资料定位：README 的 MCP 服务器配置章节和 `tool_search` 说明
- 内容：说明可选元数据字段、推荐能力分类、`tool_search` 返回匹配原因，以及找不到 MCP 工具时的回退行为。

## 7. 接入主流程

- 影响文件：`internal/app/app.go`、`internal/external/tool_search.go`
- 依赖任务：1、2、3、4、5
- 参考资料定位：`external.NewToolSearch(manager, registry)`、`ToolSearch.Definition`
- 内容：确认主流程仍只注册 `tool_search`，不提前连接 MCP server；新匹配结果无需额外接线即可进入现有 agent loop。

## 8. 端到端验证

- 影响文件：`internal/e2e/external_tools_e2e_test.go`、`internal/external/tool_search_test.go`
- 依赖任务：全部任务
- 参考资料定位：现有 `tool_search` E2E 测试、lazy discovery 测试
- 内容：覆盖配置了 docs/database 两类 MCP server 的场景，验证文档类查询优先返回 docs server、返回 matched/score/recommended，并且无效 exact select 不扫描全部 server。
