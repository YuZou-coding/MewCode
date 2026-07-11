# MewCode 版本命令验收清单

- [ ] `go test ./internal/version ./internal/command` 通过。
- [ ] `go test -count=1 ./...` 通过。
- [ ] 未注入版本的构建执行 `/version` 后输出严格等于 `MewCode dev`。
- [ ] 使用 `-ldflags "-X mewcode/internal/version.Value=v1.2.3"` 构建后，执行 `/version` 输出严格等于 `MewCode v1.2.3`。
- [ ] `/help` 输出包含 `/version`、用途说明和用法。
- [ ] 输入 `/ver` 时补全结果为 `/version`。
- [ ] `/v` 不会解析为已注册命令，未知命令提示保持现有行为。
- [ ] 版本命令不显示 Git commit、构建时间、Go 版本、操作系统或 CPU 架构。
- [ ] `README.md` 同时包含 `/version` 用法和构建时版本注入示例。
- [ ] 端到端启动 MewCode，输入 `/version` 能看到当前版本，且请求不会发送给模型。
