# MewCode MCP 延迟发现验收清单

- [x] 启动包含至少一个 MCP server 的项目后、首次模型请求前，`initialize=0` 且 `tools/list=0`。
- [x] 初始模型工具列表包含 `tool_search`，不包含任何 `external_` 前缀的远端工具。
- [x] 用关键词搜索时，结果只列出 server 名称、工具名称或描述包含该关键词的工具。
- [x] 输入 `select:external_stdio_echo` 时只连接 `stdio` server，并注册 `external_stdio_echo`。
- [x] 输入无法解析 server 的 `select:external_missing_echo` 时返回结构化错误，其他 server 的 `initialize` 计数保持 `0`。
- [x] 搜索成功后的下一轮模型工具列表包含新注册的远端工具。
- [x] 同一工具连续搜索两次只有一个注册项，server 的 `initialize=1`、`tools/list=1`。
- [x] 一个 server 发现失败时，结果的 `errors` 包含该 server，其他 server 仍可发现和调用。
- [x] 发现后的远端工具仍经过现有权限检查，并以工具结果回灌模型。
- [x] README 与全局提示词包含 `tool_search`、按需发现和精确选择说明。
- [x] `git diff --check` 无输出。
- [x] `go test -count=1 ./...` 通过且失败数为 `0`。
