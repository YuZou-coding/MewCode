# MewCode

MewCode 是一个用 Go 编写的终端 AI 编程助手。当前版本支持流式对话、ReAct Agent Loop、六个核心工具、外部工具服务器接入、上下文压缩、规划模式提示、模块化系统指令和工具执行前的纵深防御安全检查。

## 安装与任意目录启动

MewCode 推荐安装成全局命令，这样可以像 Claude Code 一样在任意项目目录运行：

```bash
cd /path/to/Mewcode
go install ./cmd/mewcode
```

确保 Go 的 bin 目录在 shell 的 `PATH` 中。临时生效：

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

永久写入 zsh：

```bash
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

安装后可以在任意目录启动：

```bash
cd /path/to/your/project
mewcode
```

MewCode 会把启动时所在目录作为项目根目录。会话、项目笔记、权限规则、Hook、Skill、worktree 等项目运行态数据会写入当前项目的 `.mewcode/`。

## 配置

MewCode 会按顺序读取配置：

1. 当前项目目录的 `mewcode.yaml`
2. 用户级全局配置 `~/.mewcode/mewcode.yaml`

项目级配置优先；如果某个项目没有 `mewcode.yaml`，会自动回退到用户级配置。因此新机器部署时，通常先准备一份全局配置：

```bash
mkdir -p ~/.mewcode
cp /path/to/Mewcode/mewcode.openai.example.yaml ~/.mewcode/mewcode.yaml
```

然后编辑 `~/.mewcode/mewcode.yaml`：

```yaml
protocol: anthropic
model: claude-sonnet-4-5
base_url: https://api.anthropic.com
api_key: your-api-key
worker_enable_verify: false
worker_background_threshold: 10s
max_iterations: 30
worktree_copy_files: settings.local.json,.env.local
worktree_link_dirs: node_modules,.venv
worktree_ttl: 7d
```

常用全局配置文件：

- `~/.mewcode/mewcode.yaml`：全局模型配置。任意项目没有自己的 `mewcode.yaml` 时会使用它。
- `~/.mewcode/MEWCODE.md`：全局用户指令。任意项目启动时都会读取，例如默认语言、工作习惯、代码风格偏好。
- `~/.mewcode/permissions.yaml`：全局权限规则。保存长期允许或拒绝的工具规则。
- `~/.mewcode/hooks.yaml`：全局 Hook 规则。用于在消息、工具、压缩等事件上触发自动动作。
- `~/.mewcode/notes.md`：全局笔记。主要保存用户偏好和纠正反馈。
- `~/.mewcode/skills/`：用户级 Skill 目录。
- `~/.mewcode/workers/`：用户级 Worker 角色目录。

配置优先级与推荐放置位置：

| 配置 | 实际优先级 | 推荐放置位置 |
| --- | --- | --- |
| `mewcode.yaml` | 项目级 `mewcode.yaml` 优先于 `~/.mewcode/mewcode.yaml` | 全局放默认模型和中转站；项目级只放特殊模型或特殊中转站 |
| `MEWCODE.md` | 项目级 `MEWCODE.md` 排在用户级 `~/.mewcode/MEWCODE.md` 前面 | 全局放个人偏好；项目级放技术栈、构建命令、项目约束 |
| `permissions.yaml` | 会话级 > 项目级 `.mewcode/permissions.yaml` > 用户级 `~/.mewcode/permissions.yaml` | 全局只放确定长期安全的 allow/deny；项目专属规则放项目级 |
| `hooks.yaml` | 用户级和项目级都会加载 | 全局放通用提醒；项目级放项目专属自动化 |
| `notes.md` | 用户级保存用户偏好/纠正反馈；项目级保存项目知识/参考资料 | 不建议互相复制，除非你明确想迁移个人偏好或项目知识 |
| `skills/` | 项目级覆盖用户级，用户级覆盖内置 | 通用 Skill 放全局；项目专属 SOP 放项目级 |
| `workers/` | 项目级覆盖用户级，用户级覆盖内置 | 通用 worker 放全局；项目专属角色放项目级 |
| `servers.yaml` | 用户级和项目级合并，项目级同名 server 覆盖用户级 | 通用 MCP/外部工具放全局；项目专属 server 放项目级 |

也可以用一键部署命令复制当前项目配置到全局：

```bash
mewcode setup-global --from /Users/theone/Documents/Mewcode
```

默认只复制 `mewcode.yaml` 和 `MEWCODE.md`。可选资源需要显式声明：

```bash
mewcode setup-global --from /Users/theone/Documents/Mewcode --include permissions,hooks,servers
```

复制全部可迁移资源：

```bash
mewcode setup-global --from /Users/theone/Documents/Mewcode --all
```

预览将要复制的内容，不写入文件：

```bash
mewcode setup-global --from /Users/theone/Documents/Mewcode --all --dry-run
```

`setup-global` 永远不会复制运行态目录：`sessions`、`artifacts`、`worktrees`、`teams`。

如果你已经在 MewCode 源码项目里配置好了 `mewcode.yaml` 和 `MEWCODE.md`，可以直接复制到全局：

```bash
mkdir -p ~/.mewcode

# 可选：先备份已有全局配置
[ -f ~/.mewcode/mewcode.yaml ] && cp ~/.mewcode/mewcode.yaml ~/.mewcode/mewcode.yaml.bak
[ -f ~/.mewcode/MEWCODE.md ] && cp ~/.mewcode/MEWCODE.md ~/.mewcode/MEWCODE.md.bak

# 把当前项目配置复制为全局默认配置
cp /Users/theone/Documents/Mewcode/mewcode.yaml ~/.mewcode/mewcode.yaml
cp /Users/theone/Documents/Mewcode/MEWCODE.md ~/.mewcode/MEWCODE.md
```

复制后，在任意目录执行 `mewcode` 都会使用这份全局默认配置；如果该目录自己有 `mewcode.yaml` 或 `MEWCODE.md`，项目级内容会覆盖或优先于全局内容。

支持的 `protocol`：

- `anthropic`
- `openai`：使用 OpenAI 兼容的 `/chat/completions` 流式接口，适合多数中转站。实际请求地址为 `base_url + /chat/completions`。

如果某个项目需要使用不同模型或中转站，在项目根目录放置自己的 `mewcode.yaml` 即可覆盖全局配置。会话历史会保存到项目内 `.mewcode/sessions/`；权限规则、Hook 规则、外部工具服务器和长期笔记使用单独的配置文件。

Agent 与 Worker 配置字段可省略。`max_iterations` 控制主 Agent 单次任务最多执行多少轮，默认 `30`；达到上限后会进行一次不携带 tools 的最终状态总结，而不是直接显示硬错误。`worker_enable_verify` 控制内置 `verify` worker 是否启用；`worker_background_threshold` 控制前台 worker 自动转后台的等待时间。

Worktree 配置字段也可省略。`worktree_copy_files` 是创建隔离目录后要复制的本地文件白名单；`worktree_link_dirs` 是要软链接的大型依赖目录；`worktree_ttl` 控制临时隔离目录过期清理时间，默认 `7d`。

## 运行

已安装全局命令后：

```bash
mewcode
```

源码目录内也可以直接运行：

```bash
go run ./cmd/mewcode
```

发布构建可在编译时注入版本号；未注入的开发构建显示 `dev`：

```bash
go build -ldflags "-X mewcode/internal/version.Value=v1.2.3" -o ./bin/mewcode ./cmd/mewcode
```

进入提示符后输入问题。输入 `/exit` 退出。助手回复会以 `MewCode >` 开头，先显示 thinking 状态和首个 token 等待时间，再按打字机效果逐字显示。若要恢复上次进入的隔离 worktree，可使用：

```bash
go run ./cmd/mewcode --resume
```

全局命令对应写法：

```bash
mewcode --resume
```

启动后会先显示 MewCode 的猫形 ASCII banner，然后进入 `You >` 输入提示符。非交互输入和自动化测试会使用脚本友好的 fallback 循环；交互式终端使用全屏 TUI，包含对话视图、输入框、Tab 补全和状态栏。

## 命令系统

MewCode 的 `/` 命令由集中式注册中心管理。命令支持名称、别名、帮助文本、用法、类型和参数提示；未知命令会提示使用 `/help`，不会发送给模型。非命令文本才会进入 Agent。

常用命令：

- `/help`、`/h`、`/?`：显示命令列表，`/help <command>` 显示单条命令用法。
- `/compact`、`/ctx`：触发上下文压缩。
- `/clear`、`/reset`：清空当前对话并开启新的会话存档。
- `/plan`：进入规划模式。
- `/do`、`/execute`：回到执行模式。
- `/sessions`、`/ls`：列出历史会话。
- `/resume <id>`、`/r <id>`：恢复指定会话。
- `/notes`、`/memory`、`/mem`：查看笔记；支持 `path` 和 `clear user|project|all`。
- `/permissions`、`/perms`：显示用户级、项目级和会话级权限规则摘要。
- `/permissions clear-session`：清空本会话临时权限规则，不修改权限文件。
- `/skills`：查看 Skill；支持 `show <name>`、`run <name> [args]`、`reload`。
- `/workers`：查看后台 worker 任务。
- `/workers show <id>`：查看 worker 任务详情。
- `/workers cancel <id>`：终止运行中的 worker 任务。
- `/worktrees`：查看当前隔离目录状态。
- `/worktrees create <name>`：创建 Git worktree 隔离目录。
- `/worktrees list`：列出隔离目录。
- `/worktrees enter <name>`：进入隔离目录并刷新项目上下文。
- `/worktrees exit`：退出隔离目录，回到主仓库。
- `/worktrees delete <name> [--force]`：删除隔离目录；有变更时需要 `--force`。
- `/commit [scope]`：运行内置 commit Skill。
- `/review [scope]`：运行内置 review Skill。
- `/test [scope]`：运行内置 test Skill。
- `/status`、`/st`：显示当前模式、session、消息数、最近 token usage、Hook 统计和 worker 统计。
- `/version`：显示当前 MewCode 版本。
- `/exit`、`/quit`、`/q`：退出。

交互式 TUI 采用轻量单列 transcript：`❯` 表示用户消息，品牌色圆点表示助手正文，工具调用在正文下方显示运行状态、目标和耗时。成功工具保持折叠；失败、权限拒绝和 Hook 拦截会自动展示原因。thinking 只显示阶段与耗时，不展示原始 thinking 内容。

输入 `/` 会打开命令面板，继续输入可实时过滤。使用 `↑/↓` 选择、`Tab` 补全、`Enter` 将选中命令填入输入框、`Esc` 关闭面板；选择命令不会立即执行，需要再次按 `Enter` 提交。带参数提示的命令会自动在末尾留出空格。长任务中可用 `PgUp/PgDn` 浏览 transcript；离开底部后新输出不会抢走当前位置，界面显示 `↓ New output`，按 `End` 返回底部并恢复自动跟随。

权限请求显示在输入框上方，包含工具、路径或命令以及触发原因。使用 `n` 拒绝、`y` 仅本次允许、`s` 本会话允许、`a` 永久允许；选择结果会作为简洁记录保留在 transcript 中。状态栏按终端宽度显示模式、消息数、Git 分支、context 字符预算占用和 token usage。

## Worker 子工作者与后台任务

MewCode 支持用统一工具 `run_worker` 启动子工作者。主 Agent 的工具列表保持稳定；模型通过参数选择预定义角色，或者不指定角色进入 Fork 模式。

Worker 角色可以放在：

- 项目级单文件：`.mewcode/workers/<name>.md`
- 项目级目录包：`.mewcode/workers/<name>/WORKER.md`
- 用户级单文件：`~/.mewcode/workers/<name>.md`
- 用户级目录包：`~/.mewcode/workers/<name>/WORKER.md`
- 内置：`explore`、`plan`、`general`；`verify` 由 `worker_enable_verify` 开关启用

同名 worker 优先级为项目级 > 用户级 > 内置。角色文件使用 YAML frontmatter + Markdown 正文：

```markdown
---
name: explore
description: 探索代码结构
tools_allow: [read_file, find_files, search_code]
tools_deny: [run_command]
model: gpt-5
max_iterations: 4
permission_mode: default
background_tools: [read_file, find_files, search_code]
---
优先读取项目结构，汇报关键文件、风险和下一步建议。
```

`model` 会真实用于创建子 worker 的 Provider；未指定时继承父模型。`tools_allow`、`tools_deny` 和 `background_tools` 会逐层收窄子 worker 可见工具，且子 worker 永远看不到 `run_worker`，避免链式嵌套。

`run_worker` 指定 `role` 时使用定义式 worker：从空白隔离会话启动，把角色正文作为环境上下文。未指定 `role` 时使用 Fork worker：继承父对话历史和父工具集合，并强制后台运行。Fork worker 的首条任务指令会要求它不能再 fork、不要请求确认、直接使用工具完成任务，并用结构化字段输出短报告。

后台任务只保存在当前进程内。完成后，MewCode 会把结构化通知注入主对话下一轮请求，不写入普通会话历史，也不会打断当前输入。

角色可以声明 `isolation: worktree`，或在 `run_worker` 参数里传入 `isolation: "worktree"`。这种模式会为子 worker 自动创建 `.mewcode/worktrees/worker/<task-id>`，把主仓库路径和 worktree 路径的翻译说明注入子 Agent。子 worker 完成后，如果 worktree 没有变更会自动清理；如果有未提交修改则保留目录，并在结果里提示。

## Git Worktree 隔离目录

MewCode 使用 Git 自带 worktree 机制提供隔离工作目录。目录固定放在项目内 `.mewcode/worktrees/`，并通过 `.gitignore` 忽略，不进入版本控制。创建前要求仓库已有 initial commit；如果当前仓库还没有任何提交，MewCode 会明确报错，不自动提交，也不退化成普通目录复制。

隔离目录名称经过严格校验：允许字母、数字、`.`、`_`、`-` 和 `/`，拒绝空段、`.`、`..`、绝对路径和路径遍历。名称 `feature/foo` 会创建目录 `.mewcode/worktrees/feature/foo`，对应分支为 `codex/feature-foo`。

创建 worktree 后，MewCode 会按配置 best-effort 初始化环境：

- 复制 `worktree_copy_files` 中存在的文件。
- 软链接 `worktree_link_dirs` 中存在的目录。
- 把子 worktree 的 Git hooks 路径指向主仓库 hooks。

删除时会先检查未提交修改和未推送提交；默认拒绝删除有风险的目录。确认要丢弃时使用 `--force`。启动时会后台清理超过 `worktree_ttl` 的安全临时目录，当前 active worktree、dirty worktree 和无法确认安全的目录都会跳过。

## Skill 系统

MewCode 支持把专项工作流封装成 Skill。启动时只把 Skill 名称和一句话说明注入模型；当模型调用 `load_skill` 或用户执行 Skill 命令时，完整 SOP 才会进入环境上下文，不写入普通会话历史。

Skill 可以放在：

- 项目级：`.mewcode/skills/<name>.md`
- 项目级目录包：`.mewcode/skills/<name>/SKILL.md`
- 用户级：`~/.mewcode/skills/<name>.md` 或 `~/.mewcode/skills/<name>/SKILL.md`
- 内置：`commit`、`review`、`test`

同名 Skill 优先级为项目级 > 用户级 > 内置。Skill 文件使用 YAML frontmatter + Markdown 正文：

```markdown
---
name: review
description: 进行代码审查
tools: [read_file, search_code]
mode: shared
model: gpt-5
context: recent
---
这里写发给模型的 SOP。
```

`model` 字段会被解析和展示，但当前章节不会切换实际 Provider/model。`tools` 会收窄模型可见工具；多个 active Skill 同时存在时取交集，系统工具 `load_skill` 始终可见。

目录型 Skill 可以带 `tools.json` 和脚本工具。脚本协议是 JSON stdin/stdout：MewCode 把工具参数写入 stdin，脚本向 stdout 输出 JSON 结果；脚本非零退出会作为结构化工具错误返回。

## Agent Loop 与工具系统

MewCode 内置六个核心工具：

- `read_file`：读取文件内容。
- `write_file`：用户确认后创建或覆盖文件。
- `edit_file`：用户确认后用原文唯一匹配替换修改文件。
- `run_command`：用户确认后执行命令。
- `find_files`：按 glob 模式查找文件。
- `search_code`：搜索文件内容并返回路径、行号和匹配行。

用户发起一次请求后，MewCode 会按 ReAct 循环运行：调用模型、接收工具调用、执行工具、把结构化结果回灌给模型，然后进入下一轮；直到模型不再请求工具或达到循环上限。

同一轮模型响应里如果包含多个工具，读类工具会并发执行，写文件、改文件和命令类工具会串行执行。工具执行过程会通过事件流回到 TUI，再渲染成终端输出。

工具执行前会经过安全检查。危险命令和项目根目录外路径会直接拒绝；未命中明确规则时会进入人在回路确认：

```text
Allow edit_file? [n] deny [y] allow once [s] allow session [a] allow always:
```

规则文件位置：

- 用户全局规则：`~/.mewcode/permissions.yaml`
- 项目固定规则：`.mewcode/permissions.yaml`
- 会话临时规则：仅保存在当前进程内

规则优先级为会话规则 > 项目规则 > 用户规则。危险命令黑名单和路径沙箱是硬边界，不能被 allow 规则绕过。

## Hooks 事件规则

MewCode 可以从用户级 `~/.mewcode/hooks.yaml` 和项目级 `.mewcode/hooks.yaml` 加载声明式 Hook。单条规则由事件、条件和动作组成；条件可省略，动作失败只记录 warning，不打断主流程。工具执行前事件支持同步拦截，拦截原因会作为工具结果回灌给模型。

示例：

```yaml
rules:
- name: block-secret-edits
  event: tool.before_execute
  all:
  - field: tool.name
    op: eq
    value: edit_file
  - field: tool.args.path
    op: glob
    value: "*.secret"
  block: "blocked by hook: {{tool.args.path}}"
  action:
    type: inject_prompt
    prompt: "Do not edit secret files."

- name: remind-before-send
  event: message.before_send
  once: true
  action:
    type: inject_prompt
    prompt: "Project reminder: prefer focused changes."
```

支持事件包括 `system.start`、`system.exit`、`system.error`、`session.start`、`session.end`、`turn.start`、`turn.end`、`message.before_send`、`message.after_receive`、`tool.before_execute`、`tool.after_execute`、`compact.before`、`compact.after`。条件操作符支持 `eq`、`not`、`regex`、`glob`；动作类型支持 `shell`、`inject_prompt`、`http` 和占位的 `sub_agent`。

## 外部工具服务器

MewCode 可以从用户级 `~/.mewcode/servers.yaml` 和项目级 `.mewcode/servers.yaml` 读取外部工具服务器列表。两级配置会合并；项目级同名 server 覆盖用户级 server。启动时会连接 server，完成 `initialize` 握手，调用 `tools/list` 发现远端工具，再把远端工具注册进现有工具中心。Agent 调用远端工具时仍然走普通工具流程，包括事件输出、权限检查和 tool result 回灌。

stdio server 示例：

```yaml
servers:
- name: local_tools
  transport: stdio
  command: /path/to/tool-server
  args: ["--project", "."]
  env:
    EXAMPLE_TOKEN: "dev-token"
  timeout_ms: 30000
```

Streamable HTTP server 示例：

```yaml
servers:
- name: remote_tools
  transport: http
  url: https://example.com/mcp
  timeout_ms: 30000
```

远端工具会使用包含 server 身份的本地名称注册，例如 `external_local_tools_query`。这样不会覆盖内置 `read_file` 等本地工具；多个 server 提供同名工具时也能区分来源。同一会话内，MewCode 会缓存每个 server 的连接和工具列表，连续调用同一远端工具不会重复握手或重新发现工具。

排查外部工具问题时，优先确认 `~/.mewcode/servers.yaml` 或 `.mewcode/servers.yaml` 是否存在、server 名称是否唯一、项目级同名覆盖是否符合预期、stdio 命令是否可执行、HTTP URL 是否可访问，以及工具是否出现在模型请求的工具列表中。

## 上下文压缩

每次调用模型前，MewCode 会按两层流程压缩上下文。本章使用字符数近似 token 数，不接入真实 tokenizer。

第一层优先处理工具结果：单个工具结果超过 12000 字符，或单轮工具结果合计超过 24000 字符时，完整结果会写入项目内 `.mewcode/artifacts/tool-results/`。对话里只保留 artifact 路径、原始大小、工具名、call id、预览和截断提示。写入失败时会保留原文，避免丢失工具结果。

第二层在历史超过 80000 字符时触发 LLM 摘要兜底。摘要复用当前模型，不携带工具列表；最近 6 轮对话保持原文，较早历史会被结构化摘要替换。摘要后会追加边界消息，提醒模型如需文件细节请重新读取，不要根据摘要脑补代码。

自动摘要连续失败 3 次后，本会话会停止自动摘要以避免死循环；第一层工具结果外置仍会继续。用户可以输入 `/compact` 手动触发双层压缩，终端会显示压缩前后消息数、字符数和生成的 artifact 数量。

## 指令模块与缓存观测

MewCode 将稳定的全局指令拆成身份、行为、工具使用、代码规范、安全边界、任务模式和输出风格等模块，并把工作目录、系统、时间、Git 状态等变化信息作为动态补充消息发送。Provider 返回缓存相关用量时，Agent 会解析为 usage 事件，便于验证提示缓存策略。

## 项目指令、会话和笔记

MewCode 启动时会读取手写指令文件，并作为内部消息注入模型请求：

- 项目级指令：`MEWCODE.md`
- 用户级指令：`~/.mewcode/MEWCODE.md`

项目级指令优先级高于用户级指令，会排在用户级指令前面。指令文件支持 `@include ./path/to/file.md` 引用其他 Markdown；include 相对当前文件解析，最大嵌套深度为 5。项目级 include 不能跳出项目根目录，缺失、循环或越界引用会产生警告并跳过。

每个会话会保存到项目内：

- `.mewcode/sessions/<session_id>/messages.jsonl`
- `.mewcode/sessions/<session_id>/meta.json`

`messages.jsonl` 采用追加写入，每行一条消息记录；恢复时坏行会跳过，后续有效行继续读取。若恢复时发现 assistant tool_use 没有匹配的 tool_result，会截断到最后完整位置。恢复出的历史过大时会先走已有上下文压缩流程；如果距离上次活跃超过 2 小时，会插入时间跨度提醒。

会话命令：

- `/sessions`：列出历史会话，显示 ID、标题、消息数和更新时间。
- `/resume <id>`：恢复指定会话并继续对话。

长期笔记分成两层：

- 用户级笔记：`~/.mewcode/notes.md`，保存用户偏好和纠正反馈。
- 项目级笔记：`.mewcode/notes.md`，保存项目知识和参考资料。

MewCode 每 6 轮对话或退出时会复用当前模型更新笔记，笔记更新请求不会携带工具列表，也不会写入普通会话历史。可用命令：

- `/notes`：查看用户级和项目级笔记。
- `/notes path`：显示两个笔记文件路径，方便手动编辑。
- `/notes clear user`、`/notes clear project`、`/notes clear all`：清空指定笔记。

## Team 小组协作

MewCode 可以在项目内维护长期存在的 Team 状态，固定保存到 `.mewcode/teams/`。每个 Team 目录包含：

- `team.json`：名称、Lead、默认后端、状态和 warning。
- `members.json`：成员花名册，包含角色、实例 ID、工作目录、后端、审批需求、状态和恢复位置。
- `tasks.json`：共享任务清单，支持 `depends_on`。
- `mailboxes/<member-id>.jsonl`：成员邮箱，坏行会跳过并产生 warning。
- `events.jsonl`：Team 生命周期事件。

成员可用 Markdown + YAML frontmatter 声明，放在 `.mewcode/teams/<team>/members/<name>.md` 或 `.mewcode/teams/<team>/members/<name>/MEMBER.md`：

```markdown
---
name: dev
role: coder
instance_id: dev-1
workdir: .mewcode/worktrees/team-dev
backend: in_process
requires_approval: true
resume_ref: alpha/dev-1
---
负责具体实现和验证。
```

Team 协作工具只会在 Lead 或 Team 成员上下文中出现，主入口 Agent、普通 Worker 和 Skill 默认看不到 `team_` 前缀工具。可用工具包括共享任务 create/get/list/update、成员消息发送，以及启动/停止成员。

运行后端：

- `in_process`：v1 真实实现，用 goroutine 启动成员任务。
- `terminal_pane`：v1 只识别配置并返回明确不支持错误，不会自动降级。

纯调度模式需要双重锁定：配置中 `team_scheduler_enabled: true`，并在 TUI 中执行 `/teams scheduler on`。开启后 Lead 只保留团队调度相关工具，并注入“理解目标、拆任务、派发、收敛结果、合并/上报”的工作流提醒。

常用命令：

- `/teams create <name>`：创建 Team。
- `/teams list`：列出 Team。
- `/teams show <name>`：查看 Team 状态。
- `/teams start <name>`：启动 Team，并把当前 actor 设为 Lead。
- `/teams stop <name>`：停止 Team。
- `/teams send <member> <message>`：给活跃 Team 成员发消息。
- `/teams status`：显示活跃 Team、Lead、成员运行数、待处理消息数、未完成任务数和调度模式。
- `/teams scheduler on|off`：切换纯调度模式。

## 测试

```bash
go test ./...
```

自动化测试使用本地假 SSE 服务和假外部工具 server，不需要真实 Anthropic、OpenAI、中转站或外部工具服务 API key。
