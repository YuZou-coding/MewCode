# MewCode MCP 标准兼容与 HTTP 认证验收清单

- [ ] `initialize` 请求的 `params.protocolVersion` 为 `2025-06-18`，`params.capabilities` 为对象，`params.clientInfo.name` 为 `MewCode`，且 `params.clientInfo.version` 为非空字符串。
- [ ] server 返回受支持的协议版本后，下一条消息是无 `id` 的 `notifications/initialized`，之后才出现 `tools/list`。
- [ ] server 返回不受支持的 `protocolVersion` 时，该 server 不进入已连接状态，错误中包含返回的版本。
- [ ] stdio 与 HTTP 测试均观察到 `initialize`、`notifications/initialized`、`tools/list` 的相同顺序。
- [ ] 配置 `headers.Authorization: "Bearer ${CONTEXT7_API_KEY}"` 时，配置校验明确拒绝该复合模板；配置 `headers.CONTEXT7_API_KEY: "${CONTEXT7_API_KEY}"` 时完整展开环境变量值。
- [ ] `CONTEXT7_API_KEY` 未设置时，发现结果包含 server 名和 `CONTEXT7_API_KEY`，MewCode 仍可继续对话和使用本地工具。
- [ ] HTTP 测试 server 在 `initialize`、`tools/list` 和 `tools/call` 请求中均收到配置的 Header。
- [ ] 认证 server 返回 HTTP 401 时，错误包含 server 名和状态码 `401`，但不包含 Header 值或测试令牌 `mewcode-secret-credential`。
- [ ] 搜索测试输出、错误文本和工具结果，`grep -r "mewcode-secret-credential"` 返回 0 条非测试夹具泄漏。
- [ ] 一个 MCP server 因配置、认证或握手失败时，另一个 server 仍能完成发现并调用工具。
- [ ] 启动含 MCP 配置的 MewCode 后，在调用发现工具前测试 server 收到的请求数为 0。
- [ ] 同一 server 连续发现两次，只出现 1 次 `initialize`、1 次 `notifications/initialized` 和 1 次 `tools/list`。
- [ ] README 的 Context7 示例只包含一个 HTTP server，并通过 `${CONTEXT7_API_KEY}` 引用认证环境变量，不包含真实密钥或 stdio 备选项。
- [ ] 设置有效 `CONTEXT7_API_KEY` 后启动 MewCode，按需发现 Context7 可看到至少 1 个远端工具，并能完成 1 次工具调用。
- [ ] `go test -count=1 ./...` 通过。
