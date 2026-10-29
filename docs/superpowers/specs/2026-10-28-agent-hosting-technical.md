# Agent 原样托管：技术实现说明

日期：2026-10-28。[产品设计](2026-10-28-agent-hosting-design.md)的实现参考。Agent 功能已在当前独立版 `github.com/cinience/skillhub` 实现；本地验证与部署边界以[实施结果](../plans/2026-10-28-agent-hosting-results.md)为准。

**实施状态：主方案已落地；本地验收完成，部署验收有环境缺口。** 阅读顺序：[实施结果](../plans/2026-10-28-agent-hosting-results.md) → [复核结论](2026-10-28-agent-hosting-review.md) → [实施任务](../plans/2026-10-28-agent-hosting.md) → [测试方案](../plans/2026-10-28-agent-hosting-tests.md)。

## 1. 哪些复用，哪些新增

复用当前 Go/Gin/GORM、SQLite/PostgreSQL、登录 Cookie/API Token、角色、限流、React/i18n、主题、登录跳转及 Vite 嵌入构建。Agent 新增独立业务数据和归档存储；Skill 的 Git、ClawHub 接口及 Bleve 搜索不变。

两种存储技术都能实现原样托管：扩展现有 Git 存储能保留文件历史，但需补齐工作树路径限制、忽略规则、无内容变化的新版本、标签重试、数据库与 Git 一致性和下载行为；本期建议直接保存 ZIP，更贴合整包分发，并能原字节返回用户上传的 ZIP。[Git archive 的属性规则](https://git-scm.com/docs/git-archive#_attributes)可能排除文件或替换正文，不可将原有 Archive 直接当作原样下载。

| 待新增文件 | 职责 |
| --- | --- |
| pkg/model/agent_package.go | AgentPackage、AgentPackageVersion 及响应类型 |
| pkg/repository/agent_package_repo.go | 查询、版本事务、latest 条件更新、软删除 |
| pkg/agentarchive/archive.go | 用标准库校验/生成 ZIP、清单、哈希、受限预览及本地归档读写 |
| pkg/service/agent_package_service.go | 权限、版本比较、发布协调、统一版本解析、错误分类 |
| pkg/handler/agent_package_handler.go | 表单、HTTP 限制、状态码、JSON 和下载附件 |
| web/src/api/agents.ts | Agent 类型与请求 |
| web/src/pages/Agents.tsx | 列表、搜索、类型筛选、分页 |
| web/src/pages/AgentDetail.tsx | 当前版本说明、文件树、预览、下载与作者操作 |
| web/src/pages/AgentPublish.tsx | 多文件、文件夹和 ZIP 上传，新增版本 |

修改 server.go 的真实初始化/路由、db.go 的 AutoMigrate、Config/YAML/env、App/Layout/翻译、Docker 和使用文档。新增测试贴近上述组件。现有 web/embed.go 已嵌入 web/static，构建走现有流程，不另起服务。不复用旧 agent_handler.go：该文件是机器账号注册，与 Agent 资产含义不同。

## 2. 数据保存在哪里

| 模型 | 字段及约束 |
| --- | --- |
| agent_packages | ID、唯一 Slug、DisplayName、Kind、Summary、Tags、OwnerID、LatestVersionID、VersionsCount、CreatedAt、UpdatedAt、SoftDeletedAt |
| agent_package_versions | ID、AgentPackageID、Version、Instructions、Changelog、ArchiveKey、ArchiveSHA256、ArchiveSize、Files、CreatedBy、CreatedAt；唯一索引 (AgentPackageID, Version) |

ID 采用现有 UUID 文本；Tags 复用 model.StringArray；Files 为 JSON 文本，包含普通文件 path/size/sha256，可附合法目录条目供树展示，不存正文。Asset 增加 OwnerID/Kind/UpdatedAt/SoftDeletedAt 索引；版本按 AgentPackageID 索引。不创建 publishing 状态记录，版本均为已完成归档的已提交版本。

ArchiveKey 由服务端 UUID 生成：`<package UUID>/<version UUID>.zip`，内部字段不返回给用户，不使用上传文件名作为宿主路径。新配置 `agentstore.base_path` 默认 `./data/agents`，`SKILLHUB_AGENT_PATH` 覆盖。临时与正式归档放同一存储卷，不落盘解压。Docker 新增 `agent-packages:/app/data/agents`，与数据库共同备份；不放在 Skill Git 镜像扫描目录。

首期按现有单实例/本地盘部署实施；多副本必须共享归档存储，不能只有共享数据库。每版本整包存储有重复占用磁盘的代价；本期不加存储驱动框架、增量去重或在线自动清理。

## 3. 上传如何保留目录并限制资源

`POST /api/v1/agents` 使用 multipart/form-data。公共字段 slug/displayName/kind/version/summary/instructions/changelog/tags；基础信息及文件均按完整快照提交，空简介、说明、标签表示清空。

| 模式 | 契约 |
| --- | --- |
| mode=files | files 为重复二进制字段；paths 为单个 JSON 字符串数组，按 files 切片顺序一一对应，数量必须一致 |
| mode=archive | 只有一个 archive 文件，必须是 ZIP；不允许同时出现 files/paths |

路径不能依赖 FileHeader.Filename。[Go multipart 文档](https://pkg.go.dev/mime/multipart#Part.FileName)明确文件名会经过平台相关的 filepath.Base，目录可能被截掉。目录选择仅去掉浏览器 webkitRelativePath 的第一层所选文件夹名；其他路径不改写，不过滤所有点文件，不用旧 Skill 上传函数。

先调用 `http.MaxBytesReader` 限整个请求 55 MiB，再 `ParseMultipartForm(8 << 20)` 设内存阈值并及时 `RemoveAll`。[ParseMultipartForm](https://pkg.go.dev/net/http#Request.ParseMultipartForm)本身不限定总请求体。500 文件上限为 multipart 元数据保留默认 part 数余量。非文件字段的 UTF-8 原始值合计最多 1 MiB（包含 paths）；tags 使用一个 JSON 字符串数组。拒绝未知字段、重复元数据字段、数量与模式不匹配。总请求体超限、multipart 元数据/part 数超限和业务字节/条目超限均为 413；格式错误为 400。解析失败时也清理已产生的 multipart 临时文件，不只在成功路径清理。

files 模式逐个读取、计数、SHA256 并写 ZIP，不构造全包 map[string][]byte。archive 模式保存受管临时 ZIP，用 archive/zip 遍历并逐条读到 EOF，检查 CRC、展开的实际字节量与文件 SHA256；通过后保留原 ZIP，不再压缩。单文件不超过 10 MiB，总内容不超过 50 MiB，500 普通文件、1000 全部条目；上传 ZIP 不超过 50 MiB，生成 ZIP 不超过 55 MiB。限制要在读取过程中实施，不只相信声明大小。

路径为有效 UTF-8 的规范 POSIX 相对路径，最多 1024 字节。拒绝控制字符、NUL、绝对路径、盘符/冒号、反斜杠、`.`/`..`、冗余斜杠、大小写不敏感的任意 `.git` 分量，以及重复路径和文件/目录同名冲突。合法目录条目可有一个末尾 `/`，规范后用于冲突检查；隐含父目录与显式目录不算冲突。ZIP 仅接受标准库支持的 Store/Deflate 普通文件和目录，拒绝链接、设备、加密、其他算法和损坏条目；不更改合法大小写不同的路径。

目录条目也检查方法、加密标记、类型和可读性；必须没有展开正文，拒绝目录名、类型标记相互矛盾的条目。至少含一个普通文件，零字节普通文件合法，仅空目录的包无效。检查前缀冲突时同时覆盖先出现 `a` 后出现 `a/b` 和相反顺序。

归档限制须在分配大量条目对象前生效：增加中央目录预检，按实际中央目录记录而非仅 EOCD 声明计数，最多 1000 条、中央目录最多 2 MiB，再交给 archive/zip 完成解析和正文校验。支持范围内的 ZIP64 按其真实目录信息检查；分卷或无法安全解释的目录整包拒绝。预检只负责边界，不自行实现解压器。避免 zip.NewReader 已分配几十万个条目后才检查数量。所有复制/校验循环检查请求 context；首期以单实例、受信登录用户为容量边界，无用户配额承诺，存储不足返回 503 并保持数据库不变。

ZIP 上传原字节返回，合法空目录及权限信息随原 ZIP 保存；文件夹上传只能保留浏览器提供的普通文件路径/内容，不能保证空目录或可执行权限。包内其他 ZIP 只是普通文件，不递归展开。前端首期只保证选择按钮上传，不实现递归拖拽目录。

## 4. 发布事务、并发和失败怎么办

正常流程：**初步权限/参数检查 → 写入并校验 ZIP → Sync/Close 并完成唯一归档保存 → 短数据库事务 → 响应。** 上传/解压处理期间不持有数据库事务。

事务内重新查资产（包括已删除记录），新资产在该事务创建；再次检查作者、Kind 与版本；插入版本，再以旧 LatestVersionID 和未删除为条件更新 latest、计数、名称/简介/标签。无旧版本时使用空值条件。条件更新影响行数为零或唯一索引冲突则回滚并返回 409。最新版本只看数据库 LatestVersionID，不按文件时间或版本创建时间猜测。

同一 slug 初次并发创建由唯一约束裁决；同一版本不覆盖。两个事务观察到同一个旧 latest 时只允许条件更新的一方完成；若后一个事务开始读取时已看到前一个提交，则允许按新 latest 校验后继续发布更高版本。不能笼统断言所有时间重叠的不同版本请求必定只有一个成功。任何提交顺序下 latest 不回退，计数与已提交版本一致。上传期间被删除的资产不能重新上架，软删除 slug 继续占用。

当前 DB 初始化没有启用 GORM TranslateError。Agent repository 局部按现有驱动类型识别错误，避免改动所有 Skill 错误语义：PostgreSQL SQLSTATE 23505 与 SQLite UNIQUE/PRIMARYKEY 为 409；SQLite BUSY/LOCKED（含 BUSY_SNAPSHOT 扩展码）、PostgreSQL 40001/40P01 为已回滚的可重试冲突 409；连接不可用为 503，未知程序错误为 500。不得按错误文案字符串判断，不得把所有数据库错误都当 409。首期服务端不自动重试发布事务；前端提示刷新版本后重试，已生成归档按事务结果处理。

**已修复的前置问题：复核时 SQLite 没有实际启用 WAL。** 当时 `db.go` 使用 `?_journal_mode=WAL&_busy_timeout=5000`，当前 glebarez/go-sqlite 驱动通过 `_pragma` 设置，旧参数不会启用 WAL；驱动自身默认 busy_timeout=5000，不能据此推断旧参数有效。隔离新库实测旧连接串得到 `journal_mode=delete/busy_timeout=5000`，`?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)` 得到 `wal/5000`。T2 已修正 DSN 构建（正确合并已有 query，不重复拼 `?`），并对真实数据库连接和池中新连接查询 PRAGMA 验证；原 Skill 闭环回归通过。

| 失败点 | 处理 |
| --- | --- |
| 文件处理失败 | 不创建数据库资产或版本；清理本请求临时文件 |
| 数据库明确回滚 | 删除本请求未引用的唯一归档，失败则记录待对账 |
| 事务提交失败、结果不明 | 保留归档；用独立、有超时的新上下文按版本 UUID 核查。查到已提交记录可恢复成功响应；未查到或查询失败均返回 503/code=publish_outcome_unknown，仍保留归档，留待停服对账。一次未查到不是未提交的证明 |
| 进程在存文件后、提交前崩溃 | 仅留下不可访问的孤立文件；无悬挂 publishing 记录，同版本可重试 |

文件保存完成后再公开数据库引用，不把数据库和文件写入假装成一个跨资源事务。[os.Rename](https://pkg.go.dev/os#Rename)在非 Unix 平台不保证原子性，因此也不能声称 Windows 上只靠 rename 即实现发布原子性。唯一 UUID 文件在入库前完成写入，发布后不覆盖。

服务端生成的唯一最终路径以独占创建方式保存，权限仅服务进程需要；保存时核对写入、ZIP Close、Sync 和文件 Close 错误。归档目录启动时不可用则启动失败；运行中缺失/损坏返回存储错误。正常请求在数据库 Commit 被调用前明确失败可清理本请求归档；进入 Commit 后遇到无法证明回滚的错误一律视为结果不明。repository 显式返回 commitUncertain，不用「调用过 Commit」等同「结果必然不明」；例如服务器明确报告事务已回滚与网络丢失 ACK 的语义不同。成功后客户端断线、响应失败均不得触发删包。这里保证进程/请求故障的一致性，不宣称覆盖所有文件系统的断电持久性；仍需数据库和归档一致备份。

本期恢复手册说明：停服确认无发布后，先备份，再清理遗留 `.tmp`，对照数据库已提交 ArchiveKey 检查孤立归档。运行中不按年龄直接删除文件，以免删掉未完成的正常发布。软删除仅改数据库，不物理清包。

## 5. API、预览与前端怎样接入

| 接口 | 契约 |
| --- | --- |
| GET /agents?q=&kind=&page=1&pageSize=20 | 返回 data/page/pageSize/total；空 data 为 []；pageSize 1–100，page 至少 1；UpdatedAt/ID 降序 |
| POST /agents | 首次 201、新版本 200，返回 agent/version；仅作者可追加版本 |
| GET /agents/:slug?version=latest | 返回 agent 和选中 version，含说明与文件清单，一次请求同步详情 |
| GET /agents/:slug/versions | 分页概要 data/page/pageSize/total，不携带所有版本完整文件清单及说明 |
| GET /agents/:slug/file?version=...&path=... | 先校验路径属于版本清单，再从 ZIP 限量读取并以 text/plain 返回 |
| GET /agents/:slug/download?version=... | 同源附件下载，application/zip，安全附件名和 X-Content-SHA256；直接发送已有 ZIP |
| DELETE /agents/:slug | 作者或管理员软删除；返回 200 JSON，兼容当前 apiFetch 的 JSON 解析 |

明确响应 DTO：列表每项仅含 id/slug/displayName/kind/summary/tags/ownerId/ownerHandle/latestVersion/versionsCount/createdAt/updatedAt，不含文件清单、说明、ArchiveKey 或完整 User。详情中的 agent 使用同一摘要结构，version 含 id/version/instructions/changelog/files/archiveSHA256/archiveSize/createdBy/createdAt；files 只列普通文件的 path/size/sha256，树的父目录由前端推导，合法空目录仍保留在原 ZIP 中。versions 每项仅 id/version/changelog/createdAt，按 CreatedAt/ID 降序，latest 以 agent 指针为准。不得按版本字符串字典序选择最新版本。用户/权限数据以现有登录身份为准，不信表单 ownerId。

接口实际前缀为 /api/v1。统一 `ResolvePackageVersion` 先检查未删除资产，再按 LatestVersionID 或明确 Version 查所属版本，不允许跨资产混用版本。归档目录不静态开放，所有详情/版本/预览/下载都走同一个解析入口。已登记归档缺失返回存储错误，不伪装成空文件或资产不存在。

状态码：404 不存在/已删除；401 未登录写操作；403 非作者；409 类型/版本/并发冲突；400 参数；413 超限；415 不支持文本预览；503 存储或数据库不可用；500 未分类内部故障。非作者追加固定为 403，不与「归属冲突」409 混用。错误 JSON 为 error/code，不暴露内部绝对路径。文本预览最多 256 KiB，有效 UTF-8 且不含 NUL 或除 TAB/CR/LF 外的控制字符，用 React pre/文本节点展示，不执行 HTML/JS。零字节文件可预览，超过 256 KiB 返回 415；非清单成员为 404；目录不能作为文件预览。

所有 Agent 读取（含错误、预览和下载）设置 Cache-Control: no-store；预览继承 nosniff，下载使用 application/zip 和安全的附件名。删除后新发起的读取不得经缓存继续公开，但已下载副本、浏览器已显示内容和已开始的下载无法追回，此边界需如实说明。GET 不做写入或删除操作。

slug 限 1–128 小写字母/数字/短横线且首尾为字母或数字，保留 publish 等静态路由名；名称 1–256 字符；简介最多 2000 字符；说明 64 KiB；更新说明 16 KiB；标签最多 20 个、每项 64 字符。版本严格 major.minor.patch，每段非负、无前导零、最大 2147483647，比较三元组，不调用当前忽略预发布后缀的比较器。

搜索 q 最多 128 字符，使用参数化、转小写的名称/slug/简介 LIKE，转义通配符；类型只能 single/suite，排序列固定。page/pageSize 是首期简单分页，不承诺更新过程中的跨页快照；不复制 Skill 的错误时间/UUID 混合游标。

分页参数只接受十进制正整数；缺省 page=1/pageSize=20，pageSize 最大 100；解析溢出或 `(page-1)*pageSize` 超出 int64 为 400。越过末页返回空数组和正确 total，不报错。SQLite 原生 lower 对非 ASCII 的大小写折叠有限：首期承诺 ASCII 大小写不敏感、中文原文子串匹配，不承诺完整 Unicode case folding；通配符 `%`、`_` 和转义符须按字面值匹配，并在两种数据库测试。

在 server.New 内注入依赖：查询与预览走现有 read 限流，发布/删除走 RequireAuth/write，下载走 download。不扩展现有 Bleve 和全站 Skill 搜索。

当前服务只有 SameSite=Lax Cookie，没有通用写请求 Origin 防护。Agent 发布/删除新增小型 Gin 中间件，调用 Go 1.25 起提供的 http.NewCrossOriginProtection().Check，在解析上传前拒绝不安全的浏览器跨源写请求并返回 JSON 403；同源 Cookie 和不带浏览器来源头的 Bearer 客户端仍可使用。不引入 CORS 通配符或默认可信来源；反向代理需保留外部 Host，部署验收覆盖。该中间件只挂 Agent 写路由，本次不重构现有认证。

前端路径 /agents、/agents/publish、/agents/:slug。先等待 auth loading 再判断发布登录；登录返回复用 safeRedirect；新版本预填元数据，但须重新提交完整包。切版本时取消或忽略过期详情/预览请求，下载始终绑定当前明确版本。apiFetchRaw 不会主动抛出 HTTP 失败，预览必须判断 response.ok。下载用同源附件链接，复用现有 Cookie。

新版本入口 `/agents/publish?slug=<slug>`；slug、kind 锁定，版本由作者填写。列表和全局导航始终显示 Agents；列表「发布 Agent」按钮对所有访客可见，未登录者进入登录页并保留目标 URL。详情追加版本仅作者可见，删除仅作者/管理员可见；删除需明确的页面内确认。切换上传模式时清空旧模式文件，避免混传；未明确成功的提交不自动清空表单或自动重发。

## 6. 已完成的基线修复与验证边界

基线修复已完成：核对真实 main/server/App 引用后，清理了 68 个未修改且不再接入独立版的旧集成文件，包括重复公共函数、旧路由、旧认证/团队/插件/存储集成和前端转发文件。修复前已将 198 个现存源码文件保存到外部快照，删除清单和本次增量差异保存在 `C:/Users/xll/.local/state/skillhub/baseline-repair/`；用户原有独立版修改保留，未重置或提交。

2026-10-28 环境已补齐：Go 1.27.3 安装在 `C:/Users/xll/AppData/Local/Programs/SkillHub/go1.27.3/go`，已加入用户 PATH；已打开且继承旧 PATH 的终端需要重开或刷新 PATH。项目根新增 `.nvmrc`，现有 Nvmx 会在项目内自动选择已安装的 Node 22.20.0 / npm 10.9.3，不修改全局 Node 符号链接。`npm ci` 和 `go mod download` 成功，go.mod/go.sum 校验值未变。TypeScript 5.9.3、Vite 8.0.3、ESLint 9.39.4 均可运行，Go 编译器及 `go test -mod=readonly -count=1 ./pkg/semver` 验证通过。

已验证：`npm run build`、`npm run lint`、`go test -mod=readonly -count=1 ./...`、`go vet -mod=readonly ./...`、`go build -mod=readonly ./cmd/skillhub`。未添加编译排除、禁用 lint 规则或跳过有效测试。Bleve 回归测试先复现过滤时排序丢失再修复；Git 归档回归先复现 core.autocrlf 导致内容变化再修复；前端搜索总数对齐后通过浏览器验证。

临时 SQLite/Git/Bleve 环境通过健康检查、登录/Cookie/whoami、两版本发布、旧版本内容读取、过滤搜索、两版 ZIP 内容相等和退出登录。浏览器通过结果计数、无结果查询切换、列表排序切换、最新技能详情检查。辅助进程和测试浏览器会话已停止。独立审阅未发现本次修复引入的重要问题。

仍未覆盖：PostgreSQL 运行与 Docker 持久化。npm audit 保留原有 13 项（10 high、2 moderate、1 low），未擅自升级依赖；依赖审计与完整环境记录位于 `C:/Users/xll/.local/state/skillhub/`。这些限制不等于 Agent 功能已实现或已完成对应运行环境验收。

后续验证顺序：基线前端构建、Go vet/test/build；归档组件往返与恶意/损坏输入测试；临时 SQLite 加临时目录验证权限、快照、并发、删除和故障注入；PostgreSQL 在测试数据库验证事务竞争；前端构建/lint 和浏览器发布、切版本、预览、下载闭环；Docker 持久化及现有 Skill 回归。不可用的工具链或 PostgreSQL 环境必须列为未验证，不能推断通过。
