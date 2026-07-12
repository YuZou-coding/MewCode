# MewCode 通用 MCP OAuth 与权限记忆任务拆解

## 1. OAuth metadata 与 401 诊断测试

- 影响文件：`internal/external/oauth_test.go`、`internal/external/http_transport_test.go`
- 依赖任务：无
- 参考资料定位：`HTTPTransport.send`、MCP Authorization `WWW-Authenticate`
- 内容：覆盖从 401 `WWW-Authenticate` 解析 `resource_metadata`，缺失 metadata 时返回不含 token 的诊断错误。

## 2. OAuth token store

- 影响文件：`internal/external/oauth.go`、`internal/external/oauth_test.go`
- 依赖任务：1
- 参考资料定位：`UserMCPServersFile` 的 home 目录处理、现有 YAML/文件权限风格
- 内容：实现按 server URL 缓存 token 的读写、过期判断、目录和文件权限。

## 3. PKCE 与 loopback callback

- 影响文件：`internal/external/oauth.go`、`internal/external/oauth_test.go`
- 依赖任务：2
- 参考资料定位：OAuth 2.1 PKCE、localhost redirect URI
- 内容：生成 verifier/challenge/state，启动 `127.0.0.1` 临时回调 server，校验 state 并取得 code。

## 4. Authorization server discovery 与动态注册

- 影响文件：`internal/external/oauth.go`、`internal/external/oauth_test.go`
- 依赖任务：1、3
- 参考资料定位：RFC9728 Protected Resource Metadata、RFC8414 Authorization Server Metadata、RFC7591 Dynamic Client Registration
- 内容：从 resource metadata 获取 authorization server，获取 metadata，若提供 registration endpoint 则注册 public client。

## 5. Token exchange、refresh 与 HTTP 重试

- 影响文件：`internal/external/oauth.go`、`internal/external/http_transport.go`、`internal/external/client.go`、对应测试
- 依赖任务：2-4
- 参考资料定位：`NewHTTPTransport`、`NewClientFromConfig`、`HTTPTransport.SendAndReceive`
- 内容：带 `resource`、PKCE verifier 和 client id 换 token；请求时加 `Authorization`；401 后刷新或重新登录并重试一次。

## 6. 配置与文档接入

- 影响文件：`README.md`、`mewcode.example.yaml`、`mewcode.openai.example.yaml`
- 依赖任务：5
- 参考资料定位：外部工具服务器章节、Context7 示例
- 内容：说明静态 Header 与 OAuth 两种认证方式；OAuth 示例不写真实 token。

## 7. 权限记忆规则修复

- 影响文件：`internal/permissions/rules.go`、`internal/permissions/permissions_test.go`、`internal/agent/agent_test.go` 或 `internal/e2e/security_e2e_test.go`
- 依赖任务：无
- 参考资料定位：`RuleForRequest`、`MatchRule`、`Agent.checkPermission`
- 内容：无路径/无命令的工具按工具名记忆；保留文件按路径、命令按命令内容的粒度。

## 8. 接入主流程

- 影响文件：`internal/external/manager.go`、`internal/external/tool_search.go`、`internal/app/app.go`
- 依赖任务：5-7
- 参考资料定位：`ToolSearch.Execute`、`Manager.Client`
- 内容：通过 `tool_search` 触发 OAuth 登录，登录完成后下一轮可使用远端工具；失败只影响对应 server。

## 9. 端到端验证

- 影响文件：`internal/e2e/external_tools_e2e_test.go`、`checklist.md`
- 依赖任务：1-8
- 参考资料定位：现有外部 HTTP MCP E2E、权限 E2E
- 内容：用本地假 OAuth server 验证 401、metadata、浏览器 URL、callback、token 缓存、tools/list、tools/call 与权限不重复询问；跑全量测试和 race 测试。
