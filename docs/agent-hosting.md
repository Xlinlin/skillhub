# Agent 与协作套件托管

从顶部 **发布 → 发布 Agent**，或 **Agents → 发布 Agent** 进入。统一发布页也可以选择发布 Skill；原 Agent 发布地址兼容跳转，新增版本时保留 slug。未登录时会先登录，再返回已选类型的发布表单。Agent 类型可选单个 Agent 或协作套件；套件可以包含产品、设计、开发和测试等多个 Agent 的原生配置、提示词和说明文件。

平台提供公开存储、版本浏览、文本预览和 ZIP 下载。作者在「使用说明」中写明适用工具、入口文件及本地运行步骤；平台不会运行上传的 Agent，也不要求特定 manifest、README 或 AGENT.md。

## 上传与版本

- ZIP：校验后保留原始 ZIP 字节和顶层目录，下载 SHA256 与上传一致；支持合法空目录。仅普通文件/目录、Store/Deflate，拒绝加密、损坏、链接和设备条目。
- 选择文件：按文件名保存，重名冲突整包拒绝。
- 选择文件夹：仅去掉所选根目录，保留相对路径、隐藏配置目录和文件内容；浏览器没有提交的空目录、可执行权限和时间戳不保证保留。
- `.agents`、`.claude`、`.gitignore` 等正常保存；任意层级 `.git`、路径穿越、绝对路径、重复或冲突路径会拒绝整个上传。包内 ZIP 只作为普通文件，不展开。

每次发布完整快照。只有作者可追加版本，已发布版本不可覆盖；版本使用严格的 `major.minor.patch`，必须高于当前最新版本。类型和 slug 首次发布后固定。名称、简介、标签显示当前资产信息，文件、使用说明和更新说明按版本保存。

作者或管理员可删除整个 Agent。删除后后续列表、详情、预览、版本和下载请求不可访问；已下载副本及已开始传输无法撤回。首期保留归档和数据库记录、保留 slug，不提供恢复页面。

## CLI 与 API 发布

进入 **发布 → Agent → CLI / API** 可查看当前站点地址的命令。使用本次构建或更新后的 CLI；原命令默认发布 Skill，Agent 必须显式设置 `--type agent`。

先运行 `skillhub login`，输入注册中心地址和 API Token。Token 由管理员在部署服务器运行 `skillhub admin create-token --user YOUR_HANDLE --label CLI` 签发；网页密码不能代替 Token。登录配置沿用 `~/.skillhub/config.yaml`，请勿提交到仓库。

```sh
skillhub login
skillhub publish ./my-agent-suite --type agent --kind suite --slug my-agent-suite --name "My Agent Suite" --version 1.0.0
skillhub publish ./my-agent-suite.zip --type agent --kind suite --slug my-agent-suite --name "My Agent Suite" --version 1.1.0
```

路径放在 `publish` 后第一位。必填 `--slug`、`--name`、`--kind single|suite`、`--version`；可选 `--summary`、`--tags "product,design"`、`--changelog`、`--instructions-file ./USAGE.md`（UTF-8，最大64 KiB）。每次提交完整快照，省略可选说明/简介/标签会提交为空；如需保留请再次传入。

目录上传保留隐藏配置及相对路径，拒绝任意层级 `.git`、链接和特殊文件；不保留空目录和可执行权限。ZIP 原字节传输，由服务端完整校验。CLI 将 multipart 流式写入受限临时文件（最多55 MiB），发送后清理，不把整个包读入内存，不跟随上传重定向，不自动重试。

网页、CLI 与 API 共用 `POST /api/v1/agents`。PowerShell 示例（Linux/macOS 把 `curl.exe` 换成 `curl`），替换地址、Token、文件路径及元数据：

```sh
curl.exe --fail-with-body "http://localhost:10070/api/v1/agents" -H "Authorization: Bearer YOUR_TOKEN" --form-string "mode=archive" --form-string "slug=my-agent-suite" --form-string "displayName=My Agent Suite" --form-string "kind=suite" --form-string "version=1.0.0" --form "archive=@./my-agent-suite.zip"
curl.exe --fail-with-body "http://localhost:10070/api/v1/agents" -H "Authorization: Bearer YOUR_TOKEN" --form-string "mode=files" --form-string "slug=my-agent-suite" --form-string "displayName=My Agent Suite" --form-string "kind=suite" --form-string "version=1.1.0" --form "paths=<./paths.json" --form "files=@./my-agent-suite/.agents/pm.md" --form "files=@./my-agent-suite/README.md"
```

执行多文件命令前，先在当前目录保存 `paths.json`（UTF-8，无 BOM），内容为 `[".agents/pm.md","README.md"]`。curl 的 `<` 将该文件作为文本字段读取；兼容 Windows PowerShell 5.1 和 PowerShell 7，避免内联 JSON 引号被原生命令参数传递破坏。

不要手动设置 Content-Type，让 curl 生成 multipart boundary。archive 模式仅传一个 `archive`；files 模式的 `paths` 为相对路径 JSON 数组，必须与重复 `files` 一一对应。两种模式不可混用。可选 `summary`、`instructions`、`changelog` 为文本，`tags` 为 JSON 数组，例如 `["product","design"]`（区别于 CLI 的逗号分隔标签）。首发201，追加版本200。

公开读取接口：`GET /api/v1/agents`（q/kind/page/pageSize）、`GET /api/v1/agents/:slug?version=1.0.0`、`GET /api/v1/agents/:slug/versions`（page/pageSize）、`GET /api/v1/agents/:slug/file?version=1.0.0&path=README.md`、`GET /api/v1/agents/:slug/download?version=1.0.0`。下载响应头 `X-Content-SHA256` 可用于核对。作者或管理员可通过需认证的 `DELETE /api/v1/agents/:slug` 删除。

## 限额与错误

最多500个普通文件、1000个ZIP条目；单文件10 MiB、内容总计50 MiB、上传ZIP50 MiB、总请求55 MiB、中央目录2 MiB、路径1024字节。使用说明64 KiB、更新说明16 KiB，所有非文件字段合计1 MiB。文本预览最多256 KiB且必须为无二进制控制字符的UTF-8文本，HTML/脚本只显示纯文本。

上传错误不会静默忽略文件。400检查字段/路径/格式，401需登录，403无操作权限，409表示重复、低版本、已删除或并发冲突，413超限，415不支持文本预览，503表示存储/数据库暂时不可用。若code为`publish_outcome_unknown`，先查该版本是否已发布，再决定重试；平台保留归档，不因一次核查未找到就删除文件。界面不会自动重发发布。

## 配置与部署

`configs/skillhub.yaml` 的 `agentstore.base_path` 默认 `./data/agents`，由 `SKILLHUB_AGENT_PATH` 覆盖。目录仅供服务使用，不能通过Web服务器静态公开。SQLite使用当前驱动支持的WAL和busy_timeout参数；默认数据库仍为`./data/skillhub.db`。

Docker Compose位于`deployments/docker/docker-compose.yml`，Agent卷为`agent-packages:/app/data/agents`，独立于Skill Git卷。保留这些命名卷重建应用容器；不要使用会删除卷的命令。首期按单实例/本地盘运行，多副本必须共享归档存储后重新验证。所有版本完整保存会重复占用空间；首期没有配额、增量去重或在线自动垃圾回收。

反向代理应保留外部Host，Agent写接口拒绝不安全的浏览器跨源请求；同源Cookie及常规Bearer客户端可用。读取响应使用no-store。命令行API路径见[技术设计](superpowers/specs/2026-10-28-agent-hosting-technical.md)。

## 备份、恢复与离线对账

1. 停止接收发布并停止应用，确认没有正在进行的上传；保留当前配置和应用版本。
2. 在同一个停服窗口备份数据库与完整Agent目录。SQLite关闭后复制数据库文件；如仍存在WAL/SHM，不单独只复制主文件。PostgreSQL使用部署方的数据库备份流程，同时保存对应时点的归档卷；Skill Git卷也纳入原有备份。
3. 恢复时先在隔离实例恢复整套数据库与归档，配置正确路径；检查每个已提交版本的ArchiveKey是否存在，大小和SHA256是否匹配。软删除资产的版本仍是有效引用，不能当垃圾删掉。
4. 对账先只生成三份清单：数据库引用且文件完整、数据库引用但文件缺失/不匹配、文件存在但无数据库版本引用。缺失归档先从配套备份恢复，不删除数据库版本来掩盖问题。
5. 只有确认服务已停、数据库是最终状态且已有备份，才能人工清理未引用的孤立ZIP或遗留临时文件。按文件年龄清理不可靠；日志中的请求失败或一次查询未找到不能替代对账。只处理已核实位于Agent根目录下的单个路径，禁止对计算出的目录直接递归删除。
6. 启动隔离恢复实例，下载多个历史版本并核对SHA256，再恢复正式服务。文件与数据库不是跨资源事务，断电持久性仍依赖文件系统及备份。

## 测试

后端测试使用真实临时数据库和归档。前端浏览器脚本为`scripts/test-agent-ui.ps1`，接收测试地址及通过环境变量提供的测试凭据，只对隔离实例运行。Windows浏览器下载使用同一Chrome的CDP鼠标点击辅助脚本，避免agent-browser的下载路径转换问题，仍需实际落盘并核对哈希。完整验收范围与执行记录见[测试方案](superpowers/plans/2026-10-28-agent-hosting-tests.md)。
