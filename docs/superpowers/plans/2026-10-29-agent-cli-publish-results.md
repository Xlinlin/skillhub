# Agent CLI / API 发布验证记录

执行日期：2026-10-29。本次增加 CLI Agent 发布与网页指引，复用既有 Agent API、认证与存储，不增加依赖或数据库迁移。

## 交付

- `skillhub publish <目录或ZIP> --type agent`：必填 slug/name/kind/version，可选 summary/tags/changelog/instructions-file；原命令默认 Skill。
- 保留隐藏配置目录、相对路径和文件字节；ZIP 原字节发送。受限临时 multipart 文件，用后清理；拒绝上传重定向，不自动重试。
- Agent 发布页面提供“上传 / CLI / API”页签；切换保留表单与文件。中英文命令、Token 获取方式、两种 multipart 模式与接口说明已补齐。
- 本地10070运行新构建，原账号与 product-delivery-demo@1.0.0 保留。源码保持当前工作区状态，没有提交、合并或推送。

## 实际验证

| 范围 | 结果 |
|---|---|
| Go 全仓测试 | 217通过，0失败，2环境跳过；`go test -json -mod=readonly ./...` |
| Go vet / 二进制构建 | 通过 |
| 前端 build / lint | 通过 |
| 实际 CLI 登录 | 隔离 USERPROFILE/HOME，未修改用户已有 CLI 配置 |
| 目录首发 / ZIP追加 | 隐藏目录、中文路径、同名不同目录、二进制、空文件逐文件一致；ZIP下载原字节及SHA256一致；旧版本未变化 |
| CLI 错误退出 | 401、非作者403、重复版本409、未知type均非零退出；无自动重试 |
| 原 Skill 命令 | 不带type实际发布成功，仍使用Skill接口 |
| 真实curl | archive首发201；files+paths追加200 |
| Windows shell | Windows PowerShell5.1和PowerShell7执行文件路径清单示例均返回201 |
| Agent指引浏览器 | 命令及协议内容、复制、中英文、字段和已选文件保留、已有Agent追加版本入口、Skill入口、390px宽度与控制台检查通过 |
| 既有网页上传回归 | 10项通过：菜单/登录、ZIP上传及下载、文本预览、新旧版本、单文件/目录、失败保留输入、搜索筛选、语言主题和手机宽度、删除、退出重定向 |

复现脚本：`scripts/test-agent-cli.py`（真正二进制及隔离SQLite）、`scripts/test-agent-publish-guide.ps1`、`scripts/test-agent-ui.ps1`（仅在隔离实例运行）。

## 审阅与修复

一次独立只读审阅发现1项重要问题：Windows PowerShell5.1会丢掉内联JSON参数中的双引号。先执行旧示例，真实API返回400；改为UTF-8无BOM的paths.json，通过curl `--form "paths=<./paths.json"`传递文本字段；两种Windows shell真实请求201验证修复。最终Go测试、vet、web build/lint、二进制构建和页面验证通过，无剩余重要问题。

测试过程中，一次新浏览器会话启动超时、一次旧UI脚本继承英文状态而在切换中文后断言英文失败。使用已初始化会话及中文起始状态后原脚本10项通过；新指引脚本已显式归一化初始语言。没有将这两次失败计为通过。

## 边界

- `TestPublishAgentRejectsSymlinks`因当前Windows用户无法创建符号链接跳过；`TestAgentPostgres`因未配置PostgreSQL跳过。Docker、PostgreSQL和竞态检测未在本次新增环境验证。
- 本机PATH中的同名`skillhub`仍是用户原有Python脚本，未替换。当前新版CLI为`C:/Users/xll/.local/state/skillhub/baseline-repair/skillhub-agent-cli-v2.exe`；本地试用需调用此文件。其他环境须构建/更新CLI后使用新参数。
