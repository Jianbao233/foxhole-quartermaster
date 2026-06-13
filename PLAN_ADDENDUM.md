# Plan 修订单（v1 → v1.1）

> 触发原因：前端栈从 HTMX/Alpine/uPlot 改为 **Vue 3 + Element Plus + ECharts**（Vite + pnpm）。
> 本文件是对 `current_plan` 的增量补丁，不替换原 Plan 主体；后续 Stage 推进以本文为准。

---

## 影响面总览

| 原 Stage | 改动类型 | 说明 |
|---|---|---|
| Stage 1 | 修改 | Go 骨架增加 `web/` 子工程占位与 embed 钩子 |
| **Stage 1.5（新增）** | 新增 | 前端 Vite 脚手架，跑通空壳 dev 模式 |
| Stage 5 | 修改 | 路由改为 `/api/*` JSON，HTML 由前端 Router 接管 |
| Stage 6 | 修改 | `/upload` 不再是后端模板页，改为前端 view + `POST /api/ingest` |
| Stage 7 | 修改 | "总览 + 单仓详情"在前端实现，后端补 list/detail JSON 端点 |
| Stage 8 | 修改 | 折线图改 ECharts，时间窗切换由前端 `dataZoom` 完成 |
| Stage 9 | 修改 | Admin 页全部前端化（Element Plus 表格 + 弹窗） |
| Stage 12 | 修改 | 发布流程：`pnpm build → go build`，产物仍是单二进制 |

工时净增估算：**约 +0.5~1 个工作日**（前端脚手架 2h + 视图阶段+0.5d，后端节省的模板时间冲抵一部分）。

---

## Stage 1（修订）：Go 骨架 + 前端工程占位

新增动作：

- 在仓库根创建 `web/` 目录（**仅占位**，下一阶段填充）
- `cmd/server/main.go` 预留 `//go:embed all:web/dist` 入口（注释形式，dist 不存在时不编译激活）
- `Makefile` 新增目标：

```
web-install:    pnpm -C web install
web-build:      pnpm -C web build
web-dev:        pnpm -C web dev
build:          web-build
                go build -o build/foxhole-quartermaster ./cmd/server
dev-back:       go run ./cmd/server
dev-front:      pnpm -C web dev
```

- `.gitignore` 增加 `web/node_modules/`、`web/dist/`、`web/.vite-cache/`
- CI workflow 增加 `pnpm/action-setup` + `pnpm -C web install` + `pnpm -C web build`，再跑 `go build`

退出条件：`make build` 能空跑过（`web/dist` 不存在时 fallback 到 stub `embed.FS`）

---

## Stage 1.5（新增）：前端脚手架

工时：~2 小时

- `pnpm create vite web --template vue-ts`（在工作区根，覆盖 Stage 1 的 `web/` 占位）
- 安装依赖：

```
pnpm -C web add vue-router pinia
pnpm -C web add element-plus @element-plus/icons-vue
pnpm -C web add echarts
pnpm -C web add -D @types/node
```

- `web/vite.config.ts` 配置：
  - `base: './'`（保证 `index.html` 内引用相对路径，便于 embed）
  - dev `server.proxy`：`/api → http://localhost:8080`、`/icons → http://localhost:8080`
  - `build.outDir: 'dist'`
- `web/src/main.ts`：挂载 ElementPlus（按需引入推荐 unplugin-vue-components 或暂时全量）+ vue-router + pinia
- 创建 router 骨架：`/login`、`/`、`/upload`、`/stockpiles/:id`、`/items/:id`、`/wars`、`/admin/*`，路由占位组件返回标题
- 写一个全局 `<el-config-provider>` 包裹根组件 + 暗色主题 import
- `pnpm -C web dev` 能打开浏览器，看到 ElementPlus 默认按钮渲染正常

退出条件：
- `web/` 工程能起 dev server
- 路由切换正常
- ElementPlus 暗色样式应用成功

---

## Stage 5（修订）：鉴权改 JSON API

- 删除原 Plan 里"`GET /login` 后端模板"路线
- 后端只提供：
  - `POST /api/auth/login` body `{steam64id, password}` → set cookie + 返回 `{user}`
  - `POST /api/auth/register` body `{steam64id, password, display_name}` → 同上
  - `POST /api/auth/logout`
  - `GET /api/auth/me` → 当前 session 用户，未登录返回 401
- 前端 `views/LoginView.vue` 用 `<el-form>` + `<el-input type="password">` 提交
- `src/api/client.ts` 在收到 401 时清空 Pinia user store + `router.push('/login')`
- 中间件 `RequireMember` / `RequireOwner` 不变，套在 `/api/*` 上

---

## Stage 6（修订）：Ingest 端到端

- 前端 `views/UploadView.vue`：
  - `<el-input type="textarea" :rows="20" placeholder="把游戏导出的 .txt 整段贴在这里">`
  - 别名输入框 `<el-input>` + `<el-checkbox>` "新建别名"
  - 点提交 → `POST /api/ingest` → 成功跳转 `/stockpiles/:id`，失败用 `<el-alert type="error">` 显示 server message
  - 提交前可点"解析预览"，后端可选提供 `POST /api/ingest/preview`（DRY-RUN，不入库）便于前端展示解析结果
- 后端 `POST /api/ingest` 行为不变（DESIGN §5.3）

---

## Stage 7（修订）：视图前端化

- 后端补 JSON 端点：
  - `GET /api/stockpiles?war_id=` → 列表（id, region, sub_region, building, alias, last_seen_at, uploader）
  - `GET /api/stockpiles/:id` → 详情（含当前态 items 列表，按 category 分组）
  - `GET /api/items/:canonical_id` → 单物品当前状态（按仓汇总）
- 前端：
  - `DashboardView` 用 `<el-table>` 渲染列表，行点击 router 跳详情
  - `StockpileDetailView` 用 `<el-table>` + `row-key="canonical_id"` + 自定义展开行渲染折线
  - 物品图标使用自定义组件 `<IconImg :item="row" />`，统一 fallback 到 `_unknown.png`

---

## Stage 8（修订）：图表与汇总

- ECharts 替代 uPlot，封装为 `components/TimelineChart.vue`：
  - props: `series` `[]{name, data: [[ts, qty], ...]}`
  - 内置 `dataZoom`：默认显示最近 24h，可拉伸到全 War
  - 暗色主题 `theme.css` 单独 register 一份 ECharts 主题
- 后端补 timeline 端点：
  - `GET /api/stockpiles/:id/timeline?item=<canonical_id>` → `{series: [{name: "散件" / "箱装", data: [[ts, qty], ...]}]}`
  - `GET /api/items/:canonical_id/timeline` → 跨仓堆叠数据
- `WarsView` 加上 War 选择器 `<el-select>`，归档 War 在 URL 上加 `?war=<id>` query

---

## Stage 9（修订）：Admin 全前端化

- 路由 `/admin/*` 在前端做 `RequireOwner` 守卫：进入前查 `GET /api/auth/me`，非 owner 跳 `/`
- `AdminLayout.vue` 用 `<el-menu mode="horizontal">` 顶部 tab：白名单 / War / 物品
- `WhitelistView` 用 `<el-table>` + `<el-input>` 内嵌编辑 + 行操作按钮
- `WarManageView` 用 `<el-steps>` 显示活跃 War + `<el-popconfirm>` 删除 + `<el-dialog>` 强制输入 `DELETE WAR <label>`
- `ItemManageView` 用 `<el-table>` 长表 + `<el-input filter>` 顶栏过滤 + `<el-upload>` 上传图标
- 后端 `/api/admin/*` 路由保持 DESIGN §5.2 设计

---

## Stage 12（修订）：发布流程

发布步骤更新为：

```
1. pnpm -C web install --frozen-lockfile
2. pnpm -C web build          → web/dist/
3. GOOS=linux GOARCH=amd64 \
   go build -ldflags '-s -w' -tags embedweb \
   -o build/foxhole-quartermaster ./cmd/server
4. tar 打包 build/* + migrations/ + seeds/ + assets/icons/_unknown.png
            + config.example.toml + systemd/* + nginx/*
5. 上传 GitHub Release
```

VPS 端**不需要装 Node**，只跑二进制。

---

## 修订后总工时

| Stage | 原工时 | 新工时 | 备注 |
|---|---|---|---|
| 0 | 1h | 1h | 不变 |
| 1 | 2~3h | 3~4h | +makefile 前端目标 |
| 1.5 | — | 2h | 新增 |
| 2 | 4~6h | 4~6h | 不变 |
| 3 | 半~1d | 半~1d | 不变 |
| 4 | 半d | 半d | 不变 |
| 5 | 半d | 半d | 后端去模板，前端补 LoginView，体量持平 |
| 6 | 1d | 1d | 同上 |
| 7 | 1d | 1.5d | 前端表格+API 联调多花半天 |
| 8 | 半~1d | 1d | ECharts 比 uPlot 配置略多 |
| 9 | 1d | 1d | Element Plus 现成组件冲抵复杂度 |
| 10 | 半d | 半d | 不变 |
| 11 | 1d | 1d | 不变 |
| 12 | 半d | 半~1d | 多一步 web build，CI 调通 |

合计 **8~10 个工作日**（原 7~9）。

---

## Todo 列表同步

新增 todo `stage-1-5`，其他 stage 描述追加"（前端 Vue3 化）"标签即可，不重排顺序。