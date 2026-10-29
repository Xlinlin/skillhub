# Agent CLI 与 API 指引实施计划

> **For agentic workers:** Use superpowers:executing-plans inline; steps track implementation and verification. User confirmed the unified publish command and shared API approach.

**Goal:** `skillhub publish <目录或ZIP> --type agent` 可实际发布，并在 Agent 发布页提供与 Skill 一致的 CLI/API 指引。

**Architecture:** 网页、CLI、curl 均调用现有 POST /api/v1/agents，不增加后端发布服务。CLI保留原Skill默认行为，Agent目录独立扫描，复用既有认证配置及归档路径校验。

**Tech Stack:** Go标准库flag/multipart/http/os.Root、现有agentarchive、React/i18n/CodeBlock；不新增依赖。

**Spec:** 用户已确认的上一条方案；原协议见[技术设计](../specs/2026-10-28-agent-hosting-technical.md)。

## Global Constraints

- 默认type=skill，显式type=agent；未知type/参数报错，不得静默发布成Skill。目录在首个位置，旗标沿用现有命令用法。
- Agent需slug/name/kind(single或suite)/version；支持summary/tags/changelog/instructions-file。不要求SKILL.md或Agent manifest。
- 目录保留隐藏配置和POSIX相对路径，拒绝.git、链接、特殊文件；500文件、10MiB单文件、50MiB总内容。ZIP按原字节传输，上限50MiB；multipart总上限55MiB。
- 复用skillhub login的Registry与Token，不新增Token管理页面、不写用户真实配置做测试。API示例只含YOUR_TOKEN占位符。
- 201/200成功；其余非零退出，保留错误code；不自动重试。上传不跟随重定向，不向其他地址转发包或Token。
- 保留既有工作区；不重置、不提交/推送混合改动。

## Review Focus

1. 目录中的隐藏文件不能被Skill旧扫描器过滤；CLI测试检查实际multipart路径和字节。
2. 输入含链接/.git/超限时不发HTTP请求；使用os.Root防止扫描后读取逃逸根目录。
3. 服务器提前拒绝或返回307时，临时multipart应清理且不重发文件到重定向目标。
4. API首次201、409冲突、503不明提交结果必须正确报告，不把首发成功当失败或擅自重试。
5. 切换上传/CLI指引不清掉已填写的上传表单，旧Skill发布仍走原接口。

## Task 1：CLI协议与命令

Files: Create pkg/cli/agent_publish.go、publish_options.go 及测试；Modify commands.go、client.go。

Interfaces: `parsePublishOptions([]string)(publishOptions,error)`；`(*Client).PublishAgent(AgentPublishOptions)(map[string]interface{},error)`；AgentPublishError含StatusCode/Code/Message；旧Publish仍默认Skill。

- [x] 写失败测试：选项兼容/未知参数拒绝，目录隐藏文件与同basename路径，ZIP原字节，201/200/409/503，提前拒绝/重定向、超限与.git，观察RED。
- [x] 实现标准flag解析、Agent独立文件收集及受限拷贝。将multipart流式写入0600受限临时文件后发送并清理，避免全包驻留内存，也避免pipe写线程在提前响应时泄漏；代价是最多55MiB临时磁盘。
- [x] 复用CLI配置，Agent请求设超时且不跟随重定向；按响应状态解码。扩展Publish分派和帮助，Skill业务路径保留。
- [x] 运行CLI测试与全仓Go测试/vet。

## Task 2：发布页与文档

Files: Create web/src/components/AgentPublishGuide.tsx；Modify AgentPublish.tsx、中英文JSON、docs/agent-hosting.md、README.md。

- [x] Agent发布页增加“上传 / CLI / API”两页签（后者合并指引，与现有Skill一致）。上传表单保持挂载，切换不丢内容。
- [x] 展示目录/ZIP CLI命令、Token来源与login步骤、PowerShell curl.exe及Bash说明、archive/files两种协议和接口表。固定占位示例不拼接未经转义的用户文本。
- [x] 构建/lint及浏览器验证指引、命令内容、切回保留字段。

## Task 3：端到端与交付

- [x] 在隔离SQLite/归档目录启动真正二进制，用隔离HOME配置执行CLI：目录首发、ZIP追加、旧新版下载哈希、401/403/409及错误退出，回归Skill命令。
- [x] 实际执行与页面一致的curl请求；记录201/200及路径/哈希。
- [x] 一次fresh只读审阅，重要问题先复现再修复；最终全仓Go test/vet、web build/lint、二进制build通过。
- [x] 更新10070展示CLI/API页签，保留本地数据，关闭测试服务；报告实际覆盖及未验证环境。
