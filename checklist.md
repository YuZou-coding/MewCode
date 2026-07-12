# MewCode 通用 MCP OAuth 与权限记忆验收清单

- [x] HTTP MCP 返回 401 且 `WWW-Authenticate` 含 `resource_metadata` 时，MewCode 会请求 resource metadata。
- [x] resource metadata 的第一个 `authorization_servers` 会被用于获取 authorization server metadata。
- [x] authorization request 包含 `response_type=code`、`client_id`、`redirect_uri`、`code_challenge`、`code_challenge_method=S256`、`state` 和 `resource=<MCP server URL>`。
- [x] token request 包含 `grant_type=authorization_code`、`code`、`redirect_uri`、`client_id`、`code_verifier` 和同一个 `resource`。
- [x] token 刷新 request 包含 `grant_type=refresh_token`、`refresh_token`、`client_id` 和 `resource`。
- [x] token 缓存在 `~/.mewcode/oauth/`，目录权限为 `0700`，文件权限为 `0600`。
- [x] 缓存命中且未过期时，不打开浏览器、不重复授权，HTTP 请求直接带 `Authorization: Bearer <token>`。
- [x] access token 过期且 refresh token 可用时，自动刷新并更新缓存。
- [x] refresh 失败或无 refresh token 时，重新触发浏览器登录。
- [x] OAuth token、refresh token、authorization code 和 client secret 不出现在错误文本、工具结果、测试日志或仓库文件中。
- [x] stdio MCP server 不触发 OAuth discovery 或浏览器登录。
- [x] 一个 OAuth MCP server 登录失败时，其他 MCP server 和本地工具仍可继续使用。
- [x] 选择 `s` 允许 `tool_search` 本会话后，同一会话内不同 query 不再重复询问。
- [x] 选择 `a` 永久允许 `tool_search` 后，重启后不同 query 不再重复询问。
- [x] 文件工具的允许规则仍按代表路径生效，不扩大到所有路径。
- [x] 命令工具的允许规则仍按命令内容生效，不扩大到所有命令。
- [x] `go test -count=1 ./...` 通过。
- [x] `go test -race -count=1 ./internal/external ./internal/rpc ./internal/agent` 通过。
