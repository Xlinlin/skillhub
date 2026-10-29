# Agent 原样托管开发与测试结果

日期：2026-10-28。**开发与本地 SQLite/浏览器验收完成；PostgreSQL 与 Docker 部署验收未完成。** 53 项是[测试设计](2026-10-28-agent-hosting-tests.md)的场景数，不代表53项已全部逐条执行；下面区分已验证、部分验证和未执行。

## 交付给用户的页面

- 服务：`http://localhost:10070/agents`，监听127.0.0.1，当前进程使用本次构建。
- 已在用户原有应用内浏览器实际表单登录 `local-admin`，菜单、列表、发布入口均可见，浏览器留在 Agents 列表。
- 本地库原来没有用户或Skill，首次启动创建本地管理员，未重置任何已有密码。初始凭据只存于当前Windows用户加密文件 `C:/Users/xll/.local/state/skillhub/local-admin-password.xml`，未写入源码或本报告。
- 已用应用内文件选择器上传 `product-delivery-demo` 1.0.0：「产品研发协作套件（示例）」，包含四个角色、README和交接说明。它用于演示原样分发，不含在线执行引擎。
- 在同一应用内浏览器实际点击下载，文件保存为 `C:/Users/xll/Downloads/product-delivery-demo-1.0.0.zip`；与上传ZIP逐字节SHA256一致：`fd98725f1e9f0443e0eabc8c2bbd3f6575f667607f9f9600e1bc3bd1d36eb532`。

## 七项任务的交付状态

| 任务 | 实现 | 已验证内容 / 边界 |
| --- | --- | --- |
| T1 归档 | 完成 | ZIP原字节、files路径/内容、隐藏目录、中文、二进制、空文件、CRC、ZIP64、中央目录预检、类型/路径/冲突/资源限制、受限纯文本 |
| T2 数据层 | 完成 | SQLite迁移可重跑并保留Skill、真实唯一索引、WAL/连接池参数、数值版本、查询/排序/分页；PG运行仍待验证 |
| T3 发布服务 | 完成 | 完整快照、身份重验、权限、版本竞争、删除竞争、明确失败清理、Commit ACK丢失/核查失败/客户端取消时保留归档、归档损坏拒绝下载 |
| T4 API | 完成 | server.New真实装配七个接口、Cookie/Bearer、Origin保护、multipart协议、请求上限/临时文件清理、状态码、DTO、no-store、指定版本下载 |
| T5 浏览页面 | 完成 | Agents导航、搜索/类型/分页、版本详情、文本预览、作者操作、中英/主题、390px无横向溢出 |
| T6 上传页面 | 完成 | ZIP/文件/文件夹、未登录回跳、新版本预填及锁定、错误保留输入、模式切换清理；真实页面与落盘下载验证 |
| T7 部署/回归 | 配置与文档完成 | 新卷/env、备份恢复手册、Compose语法及原Skill闭环通过；Docker重建/恢复、PG、race仍未通过运行验收 |

## 自动化与运行证据

最终全仓Go测试：209项通过（含子测试）、0项失败、1项显式跳过。最终Go结果以 `.superpowers/sdd/2026-10-28-agent-hosting/test-summary.json` 和 `final-test.jsonl` 为准（包含全仓原有测试及子测试，不全部属于Agent）。完整命令均实际运行并检查退出码：

| 检查 | 结果 |
| --- | --- |
| `go test -mod=readonly -count=1 -json ./...` | 通过；PostgreSQL专用测试因无DSN显式SKIP |
| `go vet -mod=readonly ./...` | 通过 |
| web `npm run build` / `npm run lint` | 通过 |
| 前端构建后 `go build -mod=readonly ... ./cmd/skillhub` | 通过，运行中的嵌入前端包含Agents页面 |
| 真实HTTP主流程重复10轮 | 通过 |
| `scripts/test-agent-ui.ps1` 对最终同一二进制 | 通过10项浏览器断言组；不是仅通过HTTP模拟登录 |
| 原Skill隔离HTTP回归 | 通过8项：ready、SPA、登录/Cookie/whoami、两版发布、旧版读取、搜索、两版下载正文、退出 |
| `docker compose ... config --quiet` | 通过；仅代表配置语法，不代表部署通过 |
| `go test -race ...` | 未执行测试，工具链报requires cgo，当前无可用gcc |
| `npm audit --json` | 保留既有13项：10 high、2 moderate、1 low；没有新增或升级依赖 |

运行二进制：`C:/Users/xll/.local/state/skillhub/baseline-repair/skillhub-agents.exe`，SHA256 `813c1823aa863ad6b9665e6bb69c78b369e06c3c30b4979cf264ba178a32db17`。

证据目录 `.superpowers/sdd/2026-10-28-agent-hosting/` 为本地忽略目录，保留以便核查，不含要提交的登录凭据：

- `final-test.jsonl`、`test-summary.json`：最终全仓测试结果；`backend-test.log`、`http-test.log`、`failure-migration.log`：分层运行记录。
- `run/ui-final/result.json`、`browser-errors.txt`、`list.png`、`detail-v1.png`、`mobile.png` 及下载ZIP：最终浏览器操作与校验；截图已打开检查。
- `npm-audit.json`：依赖审计；`changed-files.json` / `implementation.diff`：相对实施前快照的增量，复核时使用，避免混入用户原有重写。
- 原Skill回归记录：`C:/Users/xll/.local/state/skillhub/baseline-repair/agents-skill-smoke-state.json`；辅助进程已关闭。
- 实际10070列表截图：`C:/Users/xll/.local/state/skillhub/baseline-repair/agents-local-10070.png`，以未登录访客视角确认菜单和发布按钮也可见。用户应用内浏览器则保持已登录。

## 53项设计场景的实际覆盖

| 场景 | 状态 | 证据 / 尚缺部分 |
| --- | --- | --- |
| A01–A08 | 本地主路径通过 | archive/hostile测试、浏览器三模式与逐文件SHA；覆盖CRC、类型、路径、冲突、内容/单文件边界 |
| A09 | 部分 | 已测501文件、1001条目、过大中央目录、伪造EOCD；并非所有恰好上限组合都逐一运行 |
| A10 | 部分 | 空ZIP、仅目录、零字节、合法ZIP64通过；内层ZIP不递归的专门夹具未单列测试 |
| A11 | 部分 | context取消、Open/Close失败、存储目录故障及清理通过；未穷尽宿主文件Sync/磁盘短写/掉电故障 |
| A12 | 本地主路径通过 | 0/256KiB/+1、UTF8/二进制、纯文本展示；文件内容不执行 |
| D01–D03 | SQLite通过 | 迁移保留原数据、实际唯一索引、严格三段版本、同版本竞争；PG约束运行未验证 |
| D04–D06 | SQLite通过，PG未验证 | 8请求同版本、可控归档屏障下高/低版本提交次序、删除期间上传；SQLite先拿写锁，未实测PG同旧值CAS/死锁/序列化冲突 |
| D07–D08 | SQLite通过 | 同时间排序分页不重不漏、LIKE字面转义/中文/ASCII大小写；PG专用测试显式SKIP |
| D09 | 主路径通过 | page/pageSize/offset溢出、非法kind；并非每个输入字符串组合都穷举 |
| S01–S03 | 通过 | 无manifest发布、快照/旧版/同内容新版本、作者与管理员权限 |
| S04–S06 | 主路径通过 | 已删除slug不复用、高低版本次序、删除竞争、过期角色与禁用账号重验；PG并发尚缺 |
| S07–S09 | 通过 | 真实SQLite+局部driver/callback故障：插版本失败全回滚，打开/关闭/存储失败无资产，ACK丢失已提交/未找到/核查不可用/取消请求都不误删 |
| S10 | 主路径通过 | 缺失/损坏归档503、非manifest路径404、版本归属检查；未对所有DB人为损坏形态逐一注入 |
| H01–H02 | 通过 | 真实server路由、201/200、files/paths、未知/重复字段、模式混传与空集合 |
| H03–H05 | 部分 | 实测流式55MiB超限413和临时文件清理；未逐一执行所有multipart头部、字段字符和各项恰好上限组合 |
| H06–H09 | 通过 | 实际同源Cookie/Bearer、跨源403、DTO、下载头/正文哈希、所有删除后读入口与no-store |
| H10 | 部分 | 路由代码按read/write/download组接入、归档无静态挂载；未专门将每组压到429 |
| E01–E03 | 通过 | 浏览器真实登录、入口、三种上传、实际点击下载和逐文件哈希 |
| E04 | 主路径通过 | 新旧版切换与对应ZIP；取消/隔离旧请求的代码已审阅，未加入专门延迟网络的UI夹具 |
| E05 | 通过 | 搜索空态恢复、类型、中英、主题、390px布局 |
| E06 | 主路径通过 | 页面确认删除、旧URL无下载；非作者/过期管理员权限由后端实测，未单独跑第二账号UI |
| E07 | 部分 | 无效ZIP保留表单、模式切换清文件通过；未对409/413/503逐一做浏览器故障拦截 |
| E08 | 主路径通过 | 退出、发布深链登录回跳、错误态；没有穷举每种网络时序 |
| P01–P03 | 未执行 | Docker引擎启动失败，无可用PG测试DSN；不能推断兼容/持久化/恢复已经通过 |
| P04 | 通过 | 最终嵌入包实际10070页面与原Skill隔离闭环 |

## 独立审阅与修复

一次全功能独立只读审阅，未发现其他已确认的数据丢失或API/UI缺陷；发现的两项Important均以先失败后通过的回归修复：

1. 末尾`/`会使标准库给symlink/device/FIFO等条目追加ModeDir，旧条件误接受。新增 `TestRejectSlashSuffixedSpecialEntries`，检查原始Unix类型与特殊类型位，同时保留正常目录。
2. 预检与标准库选择EOCD的规则不同，注释内另一尾记录可绕过预分配条目限制。新增 `TestPreflightRejectsAlternateEOCD`，用标准库确实读出1001个条目的夹具复现；预检改为匹配同一最新候选并拒绝歧义尾部。正常ZIP64回归仍通过。

无遗留Minor审阅项。修复后重新运行全仓测试/vet及最终浏览器脚本。审阅本身未独立执行测试，其结论不能替代这里列出的运行证据。

## 实施取舍与未验证风险

- 沿用已有`codex/fix-standalone-baseline`工作区，未提交的独立版重写无法从HEAD worktree得到；先保存外部源码ZIP再增量修改，未reset/stash/提交用户改动。代价是本次代码仍需之后按增量整理提交。
- Windows使用原生命令和本地ledger替代不适配本计划中文T1–T7标题的bash任务脚本；保留每步证据。代价是进度记录需要人工核对，已做链接/覆盖自查。
- SQLite在Agent写事务首读前获取写锁，避免认证使用时间更新导致WAL快照升级失败；不改全局事务模式、不自动重发发布。代价是SQLite写操作串行，长事务仍会影响吞吐，ZIP处理在事务外。
- Windows agent-browser下载路径处理会导致Chrome取消，测试辅助脚本在同一个浏览器中用CDP设置正常路径并发出鼠标点击，仍实际落盘、核对哈希；代价是测试脚本依赖Node与CDP，不影响产品依赖。
- Docker实际安装在D盘，已尝试后台启动；其自身报错于Inference manager的`dockerInference`监听路径，未获得可用引擎。只退出本次启动的进程，未重置Docker配置/卷/数据。PG、容器重建、备份恢复仍需可用环境补验；当前PG DSN钩子也仅覆盖查询，不是完整P01。
- 无CGO编译器，race未验证；未穷尽磁盘/Sync及掉电故障。归档唯一且发布后不覆盖、提交不明保留文件是当前一致性策略，完整恢复仍依赖成套备份。
- 原Skill无关缺陷不在本次扩展范围；已跑必要回归。既有13项npm审计风险保留，不宣称整个项目已通过生产安全审计。

本地验证结束后，临时Agent测试服务、Skill回归服务和独立测试浏览器均已停止；仅保留用户验收用10070服务和原应用内浏览器页面。
