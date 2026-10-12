# 猫猫手账 Catcare

Gin + Vue 3 + GORM + PostgreSQL 的多用户、多猫健康记录与共享管理应用。默认通过 **Docker Compose** 部署，包含 nginx 前端、Gin API 与 PostgreSQL 三个服务。

## 已实现

- 账号密码注册/登录、bcrypt 密码哈希、HttpOnly 会话 Cookie、退出与24小时过期、登录限流和来源校验。
- 多个公开/私有表格；多猫档案、归档猫咪；多猫砂盆及多猫共用；猫砂类型、等待称量时间、称量说明和版本快照。
- 体重记录与30天真实数据趋势；大便干湿1–5、形态、大小1–5；尿团总重量g、个数、最大直径cm、松散度1–5；血迹、用力、无产出及备注。
- 公开表格浏览和协作申请；表主审核、按账号邀请、共同管理/仅浏览角色、权限调整与移除。公开不等于允许编辑。切换私有立即阻止未授权访问，已授权协作者保留权限。
- 累计7个独立采样日建立分版本基准，分别显示正常可分性、存放时间覆盖和异常定位验证状态。新增形成/清理时间、完整性、原始事件、长短轴/厚度及身份方式；未来共盆逐块采集无需归属，一次最多30块，更多条件折叠显示。
- 时间模型从同份已确认样本的重复测量估计老化；收缩协方差与分类分布建正常密度。批次比较正常、单猫部分变化、共同偏移及解释不足，时间假设不稳定时保留多个候选。异常定位仍未真实验证，见 [算法说明](docs/algorithm.md)。
- Gin API 所有数据与引用验证均限定表格范围；敏感配置仅放在未提交的 `.env`。前端不存储密码或会话 token。

## Docker 启动

安装并启动 Docker Desktop（Windows/macOS）或 Docker Engine + Compose v2（Linux）。

Windows PowerShell：

```powershell
.\scripts\init-env.ps1
docker compose up -d --build --wait
```

Linux/macOS：

```sh
sh scripts/init-env.sh
docker compose up -d --build --wait
```

打开 **http://127.0.0.1:8092/**。脚本会生成随机数据库密码和会话密钥，已存在的 `.env` 不会被覆盖。也可直接运行 `scripts/start-docker.ps1`。

默认仅绑定本机，数据库和 API 不暴露主机端口。PostgreSQL 数据位于 `catcare_postgres_data` 持久化卷。`docker compose down` 保留数据；不要随意使用 `down -v`，它会删除数据库卷。

可在 `.env` 配置 `BOOTSTRAP_USERNAME` 与 `BOOTSTRAP_PASSWORD` 来初始化自己的账号及空白私有表格，两者必须一起填写。已有同名账号不会被重置密码。创建成功后可移除这两项配置；数据库已保存密码哈希。仓库不包含任何真实用户的密码。

`ENABLE_DEMO=false` 默认不创建测试账号、不生成测试数据。想临时体验示例时，在开发环境设置 `ENABLE_DEMO=true` 并填写随机的 `DEMO_PASSWORD`（至少12字节）；演示账号与示例记录仅在数据库首次创建时初始化。关闭开关不会自动删除既有数据。

## 使用顺序

1. 注册自己的账号或使用已初始化账号登录；创建一个私有/公开管理表格。
2. 在“猫咪档案”添加猫咪，在“猫砂盆与猫砂”先配置猫砂，再添加猫砂盆和使用猫名单。
3. 建议前7天隔离采样，尿团、大便分别覆盖7个独立采样日，保持猫砂配置、铺砂厚度与称量方式一致。能确认时填写形成时间；同份重复称量用同一原始事件编号，测量不会重复计入样本数。同猫砂可提高基准版本重新采样。
4. 在“共享协作”邀请已注册账号，或把表格设为公开，让其他用户浏览和申请；表主决定是否授予管理权限。
5. 共享时点“共享盆采集”，整批逐块录入，不填写归属；确认完整性，按实际情况填写上次彻底清理和本次完整采集。记录页可展开“共享池批次差异”，查看各解释的证据改善；候选用于进一步排查，不单凭模型分确认身份或诊断。

## 服务端配置

所有敏感值在 `.env`，示例见 `.env.example`。Compose 传入环境变量，Go 不会覆盖已有环境变量。

| 配置 | 用途 |
| --- | --- |
| `POSTGRES_PASSWORD` | 数据库密码，推荐仅使用随机十六进制字符以避免 URL 编码问题 |
| `SESSION_SECRET` | 会话 token 的 HMAC 密钥，至少32字符 |
| `APP_ORIGIN` | 浏览器访问的精确来源，如 `http://127.0.0.1:8092`；使用 localhost 或域名时同步修改 |
| `WEB_BIND` / `WEB_PORT` | 默认 `127.0.0.1:8092` |
| `APP_ENV` | 本机 `development`；HTTPS 生产环境 `production`（开启 Secure Cookie、禁用演示） |
| `BOOTSTRAP_USERNAME` / `BOOTSTRAP_PASSWORD` | 可选的首次创建账号配置，不重置已有账号 |
| `DATABASE_URL` | 仅直接运行 Go 时需要，Docker 自动使用 db 服务生成连接地址 |

公网部署应通过外部 HTTPS 反向代理终止 TLS，设置 `APP_ENV=production`、`APP_ORIGIN=https://你的域名`、`ENABLE_DEMO=false`。外部代理转发到 web 服务；不要开放 PostgreSQL。数据库内部链路采用 Compose 网络。正式升级迁移前备份数据库。本版使用 GORM AutoMigrate；涉及破坏性 schema 变更时需另行版本化迁移。

备份（Linux 示例，账号/数据库名默认值）：

```sh
docker compose exec -T db pg_dump -U catcare catcare > catcare-backup.sql
```

## 验证

```sh
cd backend && go test ./...
cd ../frontend && pnpm install --frozen-lockfile && pnpm build
cd .. && node scripts/e2e.mjs
```

`scripts/e2e.mjs` 在运行中的应用中创建以 `qa_` 开头的独立账户，执行85项 HTTP 检查，覆盖权限、基准质量、时间/事件约束、字段持久化、整批原子保存和多批次暂定候选；记录测试用户 ID 到忽略的 `.runtime/qa-users.json`，可用 `scripts/cleanup-qa.ps1` 清理。CI 使用一次性 Docker 数据库，结束时删除其卷。不要将 E2E 指向生产环境。

GitHub Actions 在 `main` 推送时执行真实 Docker 构建、三服务健康检查、数据库 API 测试及前端入口检查。后端 Docker 构建过程也运行算法单元测试。

本地开发：Node 24、pnpm 11、Go 1.27、PostgreSQL 17。前端开发服务 `pnpm dev` 代理到 `127.0.0.1:8092`，若使用 Vite 页面写入数据，需将 `APP_ORIGIN` 设置为 Vite 的精确来源（默认 `http://127.0.0.1:5173`）。Go 直接运行时设置 `FRONTEND_DIR=../frontend/dist` 可同时提供已构建前端。

`scripts/local-db.ps1` / `start-local.ps1` 是无 Docker 环境的可选 QA 辅助，依赖本机项目内 `.tools` 的便携 PostgreSQL，不是交付部署依赖。Windows 非 ASCII 项目路径下的 QA 数据库放在 `%LOCALAPPDATA%/CatcareQA`；这些文件不进入 Git。

## 当前边界

本版没有短信验证、照片上传、设备身份自动接入或自动诊断。共享正常样本不要求归属；基线重叠、采集不完整或多个猫同时改变时可能无法定位。候选仅供排查，阈值尚无临床校准。公开表格意味着登录用户能看到该表格内猫咪与健康记录，成员账号仅对已授权成员显示。
