# Agent 原样托管 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. 本计划默认由当前会话顺序实施；不因本文自动派发子代理或提交用户已有改动。

**Goal:** 用户能从 Agents 菜单上传单 Agent 或协作套件，浏览版本并原样下载到自己的 Agent 工具。

**Architecture:** 独立 AgentPackage/AgentPackageVersion 领域，复用 Gin/GORM、现有登录和 React 页面框架。每个版本一个不可变 ZIP，完整文件先保存，再用短事务发布数据库引用；前端提供列表、详情和上传三个页面。

**Tech Stack:** 当前 Go（模块最低 1.25）、Gin、GORM、glebarez SQLite / pgx PostgreSQL、标准库 archive/zip、React 19、TypeScript、Vite、i18next。无需新增生产依赖。

**Spec:** [产品设计](../specs/2026-10-28-agent-hosting-design.md)、[技术设计](../specs/2026-10-28-agent-hosting-technical.md)、[复核修正](../specs/2026-10-28-agent-hosting-review.md)、[测试方案](2026-10-28-agent-hosting-tests.md)。冲突时以技术设计的最新补充为准。

**执行状态：** T1–T6功能与本地验收完成；T7配置、手册和Skill回归完成，PG/Docker/race未完成运行验证。[实施结果](2026-10-28-agent-hosting-results.md)逐项列出证据和覆盖缺口，以下步骤保留原始计划用于核对，不等于全部测试已通过。

## Global Constraints

- 原样托管，不运行 Agent、不定义通用 manifest；single/suite 仅分类；无强制 README/AGENT.md。
- 每版本完整快照；ZIP 上传下载 SHA256 相同；文件夹只去掉所选根目录，保留点目录与二进制。
- 请求 55 MiB；上传 ZIP 50 MiB；内容总计 50 MiB；单文件 10 MiB；500 普通文件 / 1000 ZIP 条目；生成 ZIP 55 MiB；中央目录 2 MiB；路径 1024 字节；非文件字段合计 1 MiB；文本预览 256 KiB。MiB=1048576 字节。
- slug 1–128 小写字母/数字/短横线，首尾为字母或数字，保留 publish；严格三段数字版本，每段 0–2147483647，无前导零。
- 名称 1–256 字符；简介 2000 字符；说明 64 KiB；更新说明 16 KiB；20 个标签、每个 64 字符；q 128 字符。字符限制按 Unicode 码点，KiB 限制按 UTF-8 字节。
- 公开读取；仅作者追加；作者/管理员删除；软删除保留 slug；禁止静态开放归档目录。
- Commit 结果不明保留文件；未查到不证明未提交；禁止运行时按文件年龄自动清理。
- 保留已有工作区修改和基线修复；不恢复旧 Plugin；不扩大修改现有 Skill 行为；不在源码、日志或脚本写入账号密码。
- 本计划所有功能步骤初始均未完成；设计复核、已有 Skill 基线测试不算 Agent 验收。

## Review Focus

1. 客户端在提交前后断线：已提交 ZIP 不能被清理，旧版本仍可读（T3 / S08–S09）。
2. ZIP 文件/目录冲突与伪造目录条目：不依赖输入顺序，解析前已有资源边界（T1 / A05–A10）。
3. 两个请求使用不同版本并发发布：允许合法串行提交，但 latest 永不回退（T2、T3 / D04、S05）。
4. 请求快速切换版本、筛选或上传模式：旧响应/文件不能污染当前界面（T5、T6 / E04、E07）。
5. 从 SQLite 切到部署的 PostgreSQL 17、重建容器：唯一冲突、删除和归档可读语义一致（T7 / P01–P04）。

## 文件与依赖边界

| 任务 | 新增文件 | 修改现有文件 | 独立交付物 |
| --- | --- | --- | --- |
| T1 | pkg/agentarchive/archive.go、archive_test.go（必要时拆 path/zip 校验文件） | 无 | 有资源边界、可往返验证的归档组件 |
| T2 | pkg/model/agent_package.go、pkg/repository/agent_package_repo.go、agent_package_repo_test.go、db_test.go | pkg/repository/db.go | 修正SQLite配置、可迁移并验证并发约束的数据层 |
| T3 | pkg/service/agent_package_service.go、agent_package_service_test.go | service/errors.go 仅按需增哨兵 | 发布、解析、删除与故障清理 |
| T4 | pkg/handler/agent_package_handler.go、对应测试、pkg/middleware/agent_origin.go 及测试、pkg/server/agent_routes_test.go | server.go、config.go/config_test.go、configs/skillhub.yaml | 真正挂载的完整 Agent API |
| T5 | web/src/api/agents.ts、pages/Agents.tsx、pages/AgentDetail.tsx | App.tsx、Layout.tsx、i18n/locales/zh.json/en.json、styles/global.css仅按需 | 用户可发现并浏览 Agent |
| T6 | web/src/pages/AgentPublish.tsx、scripts/test-agent-ui.ps1 | App.tsx、agents.ts、翻译、必要样式 | 三种上传与浏览器登录闭环 |
| T7 | docs/agent-hosting.md | deployments/docker/Dockerfile、docker-compose.yml、README.md | 配置、持久化、恢复与最终验收 |

依赖：T1、T2 → T3 → T4 → T5 → T6 → T7。每项完成其测试再进入下一项；本期不拆独立服务，不增加仓储接口框架。提交如有需要，仅挑选本次文件增量，不能把现有用户重写一并提交。

## T1：安全归档与原样往返

**Interfaces（新增，供 T3 调用）**

在 `pkg/agentarchive` 定义 `Entry{Path string; Size int64; SHA256 string}`、`Archive{Key string; SHA256 string; Size int64; Files []Entry}`、`InputFile{Path string; Open func() (io.ReadCloser,error)}`。目录只参与校验，Files 仅普通文件。

- `New(basePath string) (*Store,error)`：初始化并确认存储目录可写。
- `(*Store).SaveFiles(ctx context.Context,key string,files []InputFile) (*Archive,error)`。
- `(*Store).SaveZIP(ctx context.Context,key string,src io.Reader) (*Archive,error)`。
- `(*Store).Open(key string) (*os.File,error)`、`ReadText(ctx context.Context,key,path string) ([]byte,error)`、`Remove(key string) error`。
- key 仅接受内部 UUID/UUID.zip；服务层不把用户 slug/path 传给宿主路径。包内校验错误分为无效、超限、不支持预览，供服务层映射；存储错误保留内部原因用于日志。

- [x] 写 `TestArchiveRoundTrip`：内存生成含 `.agents/pm.md`、`.claude/agents/dev.md`、中文路径、同 basename、零字节与二进制文件的 ZIP；断言保存与输入 ZIP 的 SHA256 相同，逐文件路径/哈希相同，未注入文件。文件模式断言相同清单/正文，不断言 ZIP 元数据一致。
- [x] 运行 `go test -mod=readonly ./pkg/agentarchive -count=1`，确认新增测试因组件未实现而失败。
- [x] 实现以上接口，流式读取、限量+1 判断超限，校验 ZIP 关闭/CRC/真实长度、目录正文、路径冲突、中央目录实际记录预检；失败清理当前文件，不落盘解压。
- [ ] 加 A03–A12 参数化与边界测试，特别覆盖相反输入顺序、虚假 EOCD 条目数、仅目录、非法 UTF-8、伪造大小、临时文件失败与 context 取消。
- [x] 重跑组件测试，确认全部通过；检查峰值内存不随声明展开大小增长，无全包 `map[string][]byte`，无解压目录生成。保留测试日志。

## T2：模型、迁移和版本事务

**Interfaces**

`model.AgentPackage`/`AgentPackageVersion` 字段以技术设计第 2 节为准，UUID 文本、StringArray、Files 使用 json.RawMessage；ArchiveKey `json:"-"`。为 slug 和 `(agent_package_id,version)` 建命名唯一索引。`AgentPackageWithOwner` 仅补 ownerHandle。

`NewAgentPackageRepo(db *gorm.DB) *AgentPackageRepo`；新增：

- `GetBySlug(ctx,slug)` 只查未删除；`GetBySlugIncludingDeleted(ctx,slug)` 专供发布检查；二者返回 `(*model.AgentPackageWithOwner,error)`，不存在为 nil,nil。
- `List(ctx context.Context,opts AgentListOptions) ([]model.AgentPackageSummary,int64,error)`，opts 为 Q/Kind string、Page/PageSize int；摘要含技术设计明确的字段及 latestVersion。
- `GetVersion(ctx context.Context,packageID uuid.UUID,version string) (*model.AgentPackageVersion,error)`；`GetVersionByID(ctx,id uuid.UUID)` 同返回类型。
- `ListVersions(ctx context.Context,packageID uuid.UUID,page,pageSize int) ([]model.AgentVersionSummary,int64,error)`。
- `CommitPublish(ctx context.Context,actor *model.User,agent *model.AgentPackage,version *model.AgentPackageVersion) (created bool,commitUncertain bool,err error)`：事务内重读资产、验证身份/类型/递增版本、创建或取真实资产 ID、插版本并 CAS latest。候选 agent.ID 必须与归档 key 一致；若新 slug 被他人抢先创建则 409，不能把已准备的 ZIP 改挂到另一 package UUID。只有Commit结果无法证明提交或回滚时返回 `commitUncertain=true`；成功或明确回滚时为false。上层仅在err非空且该值为false时清理候选归档，成功一律保留。
- `SoftDelete(ctx context.Context,actor *model.User,slug string) error`：事务内验证并更新未删除资产，不删除归档。

版本严格解析与比较用一个明确的三元组 helper，可放 repository 同包供事务校验；service 预检复用导出的 `ParseAgentVersion(string) ([3]int64,error)`，不复用宽松 semver。

- [x] 写真实临时 SQLite 的 `TestAgentMigration`、`TestAgentPublishConstraints`、`TestAgentList`：迁移可重跑，用户/Skill 数据保留；唯一约束生效；已删除 slug 占用；列表摘要不泄露清单/ArchiveKey。运行 `go test -mod=readonly ./pkg/repository -run Agent -count=1`，确认失败。
- [x] 在db_test.go写 `TestSQLiteConnectionPragmas`：通过NewDB打开真实临时文件库，取至少两个独立池连接，分别断言journal_mode=wal、busy_timeout=5000；已有query参数可正确合并。先运行确认当前旧DSN不能满足WAL断言，再把db.go改为驱动支持的 `_pragma=journal_mode(WAL)`、`_pragma=busy_timeout(5000)`（正确构造/合并query），重跑测试通过。不要只修测试helper。
- [x] 实现模型、AutoMigrate、查询及 CommitPublish；只在短事务中访问数据库，不做 ZIP I/O；所有修改含未删除条件，latest 比较使用 IS NULL 或实际 UUID。
- [x] 实现局部驱动错误分类：SQLite 使用当前 glebarez/go-sqlite 的 Code，PG 使用 pgconn.PgError.Code，按技术设计映射。未知错误不转为冲突，不全局启用 TranslateError。
- [ ] 加 D03–D09：用两个真实连接和同步屏障制造同前值 CAS、首次 slug、同版本、版本高低顺序、删除竞争；断言行数、VersionsCount、LatestVersionID 一致。普通 goroutine 同时启动不算确定性竞争测试。
- [x] 运行本任务测试通过；PostgreSQL 同一组测试保留 `SKILLHUB_TEST_POSTGRES_URL` 入口，环境缺失必须显式 skip 并列为未验证。

## T3：发布协调、权限和故障恢复

**Interfaces**

新增 `NewAgentPackageService(repo *repository.AgentPackageRepo,store *agentarchive.Store) *AgentPackageService`。`AgentPublishRequest` 包含 Slug/DisplayName/Kind/Version/Summary/Instructions/Changelog string、Tags []string、Mode string、Files []agentarchive.InputFile、Archive io.Reader。

- `Publish(ctx context.Context,user *model.User,req AgentPublishRequest) (*AgentPublishResult,error)`，结果 Agent/Version/Created；首次创建 201 由 handler 选择。
- `ResolvePackageVersion(ctx context.Context,slug,version string) (*model.AgentPackageWithOwner,*model.AgentPackageVersion,error)`；空 version 视 latest，明确版本只在当前 package 内查询。
- `List(ctx,opts)` 与 `Versions(ctx,slug,page,pageSize)` 使用 T2 查询结果；`ReadText(ctx,slug,version,path)` 返回 []byte；`Download(ctx,slug,version)` 返回已打开 `*os.File` 及版本信息，由 handler 关闭；`Delete(ctx,user,slug)`。
- 复用 ErrValidation/ErrForbidden/ErrNotFound/ErrConflict；仅按需补 ErrTooLarge/ErrUnsupportedPreview/ErrUnavailable 和提交结果不明的稳定 code，handler 不判断文案。

- [x] 写 `TestAgentPublishSnapshots`、`TestAgentPermissions`：无特殊 manifest、同内容可发更高版本、1.1.0 完整快照不遗留旧文件，旧版不变；非作者追加 403，管理员也不能冒充作者追加。运行 `go test -mod=readonly ./pkg/service -run Agent -count=1` 确认失败。
- [x] 实现参数预检 → 准备唯一 package/version UUID → 保存完整 ZIP → 调用 CommitPublish → 生成 DTO。明确回滚失败清理；commitUncertain=true时保留并使用独立超时 context（建议 3 秒）核查版本 UUID；核查到则返回成功，否则 503/code=publish_outcome_unknown。按typed错误确认回滚的竞争返回409，不与网络ACK丢失混用。
- [x] 实现统一版本解析、预览成员校验、下载 open 和软删除；不信任请求 ownerId，不向前端返回宿主路径或完整 User。
- [ ] 增加 S04–S10 故障测试：存储短写/关闭失败、事务插入失败、CAS 失败、提交成功但 ACK 丢失、核查未找到、核查不可用、响应断开、删除竞争。优先用真实 SQLite 和 GORM callback；Commit ACK 场景仅在测试中用最小 driver 包装注入，避免新增生产故障框架。
- [x] 运行 service、repository、agentarchive 相关测试全部通过；核对「可见版本必有归档」及「已提交归档绝不清理」两个不变量。

## T4：API、实际路由与配置

**Interfaces**

新增 AgentStoreConfig/BasePath，默认 `./data/agents`，YAML `agentstore.base_path`，env `SKILLHUB_AGENT_PATH`。`NewAgentPackageHandler(svc *service.AgentPackageService)` 提供 List/Publish/Get/Versions/GetFile/Download/Delete Gin 方法，路径/DTO 采用技术设计第 5 节。`middleware.AgentOriginProtection() gin.HandlerFunc` 调标准库 Check，拒绝时 JSON403。

- [x] 写 `TestAgentRoutes`：通过真实 `server.New` 构造服务，并用临时配置和 `httptest` 请求验证 GET /api/v1/agents 返回 JSON 数组及 POST 未登录401；不是直接绑定 handler 的假路由。测试放 server 包内以访问 router。新增配置默认/YAML/env 优先级测试。运行 `go test -mod=readonly ./pkg/server ./pkg/config -run Agent -count=1` 确认失败。
- [x] 新增配置，初始化 Store/repo/service/handler 并挂载所有七条 API；读/写/下载各使用现有对应限流组，Agent 写路由增加 Origin 防护。归档路径不能 static mount。
- [x] 实现 multipart 严格解析：MaxBytesReader、8 MiB 内存阈值、非文件值合计 1 MiB、单值元数据、tags/paths JSON 数组、模式互斥、未知字段拒绝和全路径清理。用 InputFile.Open 适配文件头，不提前读全包。
- [x] 实现统一错误 JSON/code、分页、no-store、纯文本预览、安全附件名和 SHA256 头；下载只用已打开文件/响应流，打开失败在写200前返回503，避免文件发送 helper 把存储缺失变404。
- [ ] 写 H01–H10 HTTP 测试，使用真实登录 Cookie 和 Bearer 两条链；覆盖真实 router 的 read/write/download 路由、超限和未泄漏路径。运行 `go test -mod=readonly ./pkg/handler ./pkg/middleware ./pkg/server ./pkg/config -count=1` 通过。

## T5：导航、列表和版本详情

**Interfaces**

`web/src/api/agents.ts` 定义 `AgentSummary`、`AgentVersion`、`AgentDetail{agent,version}`、`AgentPage<T>{data,page,pageSize,total}` 与服务端逐字段一致；导出 `listAgents(params,signal?)`、`getAgent(slug,version,signal?)`、`listAgentVersions(slug,page,pageSize,signal?)`、`getAgentFile(slug,version,path,signal?)`、`deleteAgent(slug)`、`agentDownloadURL(slug,version)`。复用 apiFetch/apiFetchRaw，所有 URL 参数 encode；预览检查 response.ok。

- [x] 创建前端接口，App 添加 `/agents`、`/agents/:slug`，Layout 增常显 Agents 导航；补中英文文案。列表发布按钮链接 `/agents/publish`，T6 注册页面；本任务不宣称完整上传功能可验收。
- [x] 实现列表请求加载/错误/空态、关键词和类型筛选、page/pageSize/total 分页，改变筛选回第一页；取消或忽略过期请求。卡片显示作者、类型、简介、latestVersion、更新时间。
- [x] 实现详情、版本分页选择、使用说明和更新说明、按 manifest 推导的文件树及纯文本预览；每次切版本清预览，选中版本请求成功后才开放其下载链接，过期请求不能覆盖当前状态。
- [x] 作者可见新增版本入口；作者/管理员可见删除和二次确认；删除后回列表。404、415、503 分别显示明确错误；文本不经 HTML/Markdown 执行。
- [x] 运行 `npm run build`、`npm run lint`（web 目录）；用 T4 API 准备临时样例，浏览器验证 E01、E04–E06 的导航、筛选、版本、预览、删除，记录结果。无测试数据时不以空页代替详情验收。

## T6：发布页和真正的页面测试

**Interfaces**

AgentPublish 注册 `/agents/publish`，读可选 `?slug=`；调用 `publishAgent(form: FormData): Promise<AgentDetail>`。files 模式按文件选择顺序提交 files 和 paths JSON；folder 使用 webkitRelativePath 去第一层；ZIP 使用 archive；tags JSON。已有资产锁 slug/kind，预填当前名称/简介/标签/说明，版本和完整文件快照重新填写。

- [x] 实现 auth loading/未登录跳转，保留完整发布目标；三种选择按钮、必要字段、限制提示和文件清单，不过滤隐藏文件、不实现未声明的目录拖拽。
- [x] 实现上传模式互斥、切模式清理、提交中禁用、错误保留输入、成功导航明确版本详情；409 提示刷新最新版本后重试，503 结果不明提示核查版本，均不自动重发。
- [x] 编写可重复 `scripts/test-agent-ui.ps1`：针对隔离端口/临时数据目录，凭据从环境注入，驱动已安装 agent-browser；实现时先检查 CLI 支持的上传/下载命令，不把 HTTP 脚本冒充浏览器操作。截图和结果输出到临时运行目录，不保存会话密码或 token。
- [ ] 执行 E01–E08：浏览器表单登录→Agents菜单→上传 suite ZIP→列表→详情预览→点击下载→校验 ZIP SHA256；再测文件和文件夹模式、旧新版切换、失败重试状态、第二账号权限及退出后入口。实际检查截图和浏览器错误；HTTP 可补充校验，但不能替代页面点击。
- [x] 重跑 build/lint，修复真实交互问题，记录浏览器版本、用例、实际结果和下载文件哈希；未执行项明确列出。

## T7：部署、恢复与最终回归

**Files:** 修改 `deployments/docker/Dockerfile`、`deployments/docker/docker-compose.yml`、README；新增 `docs/agent-hosting.md`。

- [x] Docker 创建 Agent 数据目录，Compose 增 `agent-packages:/app/data/agents` 卷和 `SKILLHUB_AGENT_PATH`；不更改现有用户卷名称。文档说明公开性、格式/大小限制、单实例、三种上传差异。
- [x] 写停服备份/恢复/对账步骤：先停发布和服务，数据库与归档同一备份点，列出已引用/孤立/缺失文件并人工核查后处理；软删除归档仍算引用。首期只提供手册，不增加在线自动 GC。
- [ ] 执行 P01–P04：PostgreSQL17 实测事务竞争；Docker 发布后重建应用容器（保留卷）、旧版本仍可下载且 SHA256 相同；从配套备份恢复验证。没有 Docker/PG 就报告未验证，不能按 SQLite 结果推断。
- [x] 执行全量门禁：`go test -mod=readonly -count=1 ./...`、`go vet -mod=readonly ./...`、web build/lint，先构建前端再 `go build -mod=readonly -o <临时输出路径> ./cmd/skillhub`；有可用 CGO 工具链时再运行相关包 `go test -race`，不可用则单列。
- [x] 实测现有 Skill 登录、发布、搜索和两版本下载回归。测试在隔离目录，不向用户正式库写测试垃圾，不覆盖已有密码。
- [x] 最终展示本地新二进制前先确认当前进程路径和数据配置，保留数据并更换进程，访问实际10070检查菜单、入口、发布及指定版本下载。记录被测试的构建信息，避免测试新包但给用户看旧进程。
- [x] 对照测试方案填写通过/失败/未执行，检查本次增量 diff，审阅权限/归档/事务关键路径；Agent 功能未实现或任一核心闭环失败时不能标完成。

## 计划自查

范围映射：类型/元数据 T2–T3；路径/原样/资源 T1；权限/版本/删除 T2–T4；接口/限流/配置 T4；导航/浏览 T5；三种发布/真实登录 T6；部署/恢复/旧功能 T7。五项 Review Focus 均有指定测试，实际执行与部署缺口见[实施结果](2026-10-28-agent-hosting-results.md)。代码和文档仍保留在当前工作区，未提交用户已有改动。
