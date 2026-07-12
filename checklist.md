# MewCode 通用 MCP 工具路由验收清单

- [x] `mcp_servers.yaml` 支持可选字段 `description`、`capabilities`、`keywords`、`examples`，旧配置不修改也能通过现有测试。
- [x] 配置 `capabilities: ["docs"]`、`keywords: ["react", "library documentation"]` 后，查询 `React useEffect 最新文档` 只连接匹配 docs server，不优先连接无关 database server。
- [x] `tool_search` 返回的每个候选工具包含 `server`、`name`、`description`、`capabilities`、`matched`、`score`、`recommended` 字段。
- [x] 当多个候选匹配时，最高分候选 `recommended=true`，其他候选 `recommended=false`。
- [x] `select:external_<server>_<tool>` 仍只连接名称对应的 server；无法解析 server 时返回 `tool_not_found`。
- [x] 无元数据的 MCP server 仍可通过 server 名、远端工具名或远端工具描述被发现。
- [x] 稳定系统提示包含通用 MCP 路由规则，明确最新/官方文档、外部 SaaS/API、浏览器实时状态、数据库、Issue 系统、云服务、项目外知识库应优先考虑 `tool_search`。
- [x] 找不到合适 MCP 工具时，提示要求模型明确说明未发现合适工具，不允许假装查过外部来源。
- [x] README 展示带元数据的 `context7` 示例，并说明元数据只是路由提示，不会触发启动时连接。
- [x] E2E：配置 docs 与 database 两个 fake MCP server，输入文档类搜索后，测试观察到 docs server 被发现并返回 recommended 候选，database server 不因粗筛被连接。
- [x] `go test -count=1 ./...` 通过。
