# MewCode 通用 MCP OAuth 与权限记忆规格

## 背景

MewCode 已支持 stdio 与 HTTP MCP tools 基础流程，但 HTTP MCP 只能使用静态 Header 认证。Context7 这类遵循 MCP Authorization 规范的 server 可通过 OAuth 登录，不应要求用户手动维护 API key。与此同时，用户选择本会话允许或永久允许某类工具后，MewCode 不应因为无关参数变化反复询问相同权限。

## 目标用户

- 使用需要 OAuth 登录的 HTTP MCP server 的 MewCode 用户。
- 需要同时接入多个标准 HTTP MCP server 的开发者。
- 希望权限确认结果符合“工具、路径、命令”直觉边界的终端用户。

## 能力清单

- HTTP MCP server 返回 OAuth 401 时，MewCode 可按 MCP Authorization 规范发现授权信息。
- MewCode 使用浏览器登录、本地 loopback 回调和 PKCE 完成 OAuth 授权码流程。
- OAuth token 按 server URL 缓存在 `~/.mewcode/oauth/`，重启后可复用。
- access token 过期时优先使用 refresh token 刷新；刷新失败时重新触发浏览器登录。
- 后续 HTTP MCP 请求自动携带 `Authorization: Bearer <token>`。
- OAuth token、授权码、refresh token、client secret 不进入日志、错误、工具结果或仓库。
- stdio MCP 不走 OAuth 流程，继续由命令环境自行处理凭据。
- 对没有路径和命令边界的工具，本会话允许/永久允许按工具名记忆，避免同一工具因参数变化重复询问。
- 对文件工具仍按代表路径记忆；对命令工具仍按命令模式记忆。

## 非功能要求

- OAuth 实现遵循 MCP `2025-06-18` Authorization 规范中的 OAuth 2.1、Protected Resource Metadata、Authorization Server Metadata、PKCE 和 Resource Indicators 要求。
- OAuth 客户端不写死 Context7；server URL、metadata、client registration、token endpoint 都通过标准发现得到。
- token 缓存文件权限为 `0600`，目录权限为 `0700`。
- 用户取消登录、浏览器打开失败、metadata 缺失、动态注册失败、token 刷新失败都返回可诊断但不含密钥的错误。
- 权限记忆规则必须有自动化测试覆盖，证明 `tool_search` 参数变化不会重复询问。

## 设计骨架

- `internal/external/oauth.go` 负责 OAuth discovery、PKCE、loopback callback、token exchange、refresh 和 token store。
- `HTTPTransport` 在收到 401 且存在 OAuth metadata 时调用 OAuth provider 获取 token，并带 token 重试一次请求。
- OAuth token store 以 server URL 的 SHA-256 作为文件名，避免路径非法字符和泄漏 server 细节。
- `ServerConfig` 保持静态 headers 兼容；未配置 headers 时也可通过 401 自动 OAuth。
- 权限规则生成仍集中在 `permissions.RuleForRequest`，仅调整无路径/无命令工具的 fallback 行为。

## 不做范围

- 不为 stdio transport 实现 OAuth。
- 不实现 MCP resources/prompts 的一等能力。
- 不实现多个 authorization server 的交互式选择；默认选择 metadata 中第一个。
- 不实现手动粘贴 token/code 的备用 UI。
- 不把 token 同步到项目配置或会话文件。

## 完成定义

见 [checklist.md](checklist.md)。
