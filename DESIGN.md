# Foxhole 仓管系统 — 设计文档

> 工作区：`J:\Developing\Foxhole_project`
> 部署目标：Ubuntu VPS（已部署其他应用，资源占用要尽可能小）
> 文档约定：本文件是**实施前的最终设计**，写代码以此为准；后续如改动需先更新本文。

---

## 1. 目标与边界

### 1.1 业务目标
- 把游戏内导出的仓库数据（如 `样本.txt`）汇总到一个军团总账户名下
- 不在游戏里的玩家通过网页查看军团整体物资状态、单仓存量、历史变化
- 美术资源复用游戏内/Wiki 的物品图标

### 1.2 范围内
- 单军团 + 多成员（成员账号体系）
- 单 War 实时数据 + 历史 War 静默归档
- 文本格式 ingest（粘贴/上传 .txt）
- 网页只读浏览 + 折线图 + 上传流水

### 1.3 范围外（v1 不做）
- 多军团 SaaS
- 桌面 watcher / Discord bot 上传
- 地图坐标可视化（V2 加分项，数据先存）
- 移动端原生应用

---

## 2. 角色与权限

| 角色 | 数量 | 来源 | 能做 |
|---|---|---|---|
| `owner` | 1 | 部署时由配置文件指定一个 Steam64ID | 全部权限：管理白名单、开新 War、归档 War、改密、踢人 |
| `member` | N | owner 在白名单里加 Steam64ID 后，玩家自助注册激活 | 上传 ingest + 查看全部数据 + 改自己昵称/密码 |
| `guest` | 0 | 未登录访问任何业务页面 → 跳登录页 | 仅访问登录/注册页 |

### 2.1 注册流程（open_register_whitelist）

```
[owner 后台]                      [新成员]
  │                                  │
  ├─ 添加 Steam64ID 到白名单          │
  │  + 设置初始 display_name (可选)   │
  │                                  │
  │  ───── 把 ID 告诉成员 ─────►      │
  │                                  │
  │                                  ├─ 访问 /register
  │                                  ├─ 输入 Steam64ID + 自定义密码 + 显示昵称
  │                                  ├─ 服务端校验：白名单里有此 ID 且未注册过
  │                                  ├─ argon2id 哈希入库
  │                                  └─ 自动登录 → /dashboard
```

**关键点**：
- Steam64ID 不验证真实归属（trust_owner 策略）
- 同一 Steam64ID **只能注册一次**；密码忘了由 owner 在后台触发"重置标记" → 该用户下次访问 `/register` 时可重新设密
- owner 没有"系统生成初始密码"流程，简化实现

### 2.2 吊销
owner 在白名单页面点"踢出"：
- `users.is_active = 0`
- 该用户已存在的 session 全部失效
- **不级联删** snapshots，保留历史署名（前端显示 `[已踢出] xxx`）

---

## 3. 数据模型（SQLite Schema）

### 3.1 表结构

```sql
-- 军团成员
CREATE TABLE users (
  steam64id        TEXT PRIMARY KEY,        -- 17 位数字字符串
  display_name     TEXT NOT NULL,           -- 玩家昵称，可改
  password_hash    TEXT,                    -- argon2id；NULL 表示已加白名单未注册
  is_owner         INTEGER NOT NULL DEFAULT 0,
  is_active        INTEGER NOT NULL DEFAULT 1,
  must_reset_pw    INTEGER NOT NULL DEFAULT 0,  -- owner 触发的重置标记
  created_at       INTEGER NOT NULL,        -- unix 秒
  registered_at    INTEGER                  -- 首次完成注册时间
);

-- 登录 session（不用 JWT，避免吊销不及时）
CREATE TABLE sessions (
  token        TEXT PRIMARY KEY,            -- 32 字节随机 hex
  steam64id    TEXT NOT NULL REFERENCES users(steam64id),
  created_at   INTEGER NOT NULL,
  expires_at   INTEGER NOT NULL,
  last_seen    INTEGER NOT NULL,
  user_agent   TEXT
);
CREATE INDEX idx_sessions_uid ON sessions(steam64id);

-- 战争（War）
CREATE TABLE wars (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  label        TEXT NOT NULL,               -- 例：'War 124'，由 owner 输入
  started_at   INTEGER NOT NULL,
  ended_at     INTEGER,                     -- NULL 即 active
  is_active    INTEGER NOT NULL DEFAULT 0,  -- 全表只能有一行 1（DB 触发器保证）
  notes        TEXT
);

-- 物品主数据
CREATE TABLE items (
  canonical_id TEXT PRIMARY KEY,            -- 内部稳定 ID，例 'mat_basic_materials'
  name_zh      TEXT NOT NULL,               -- 中文名（与导出文本一致，去 ' (箱装)' 后缀）
  name_en      TEXT,                        -- 英文名（用于 Wiki 图标查找）
  faction      TEXT,                        -- 'colonial' / 'warden' / 'neutral'
  category     TEXT,                        -- 'rifle' / 'ammo' / 'vehicle' / 'material' ...
  crate_size   INTEGER,                     -- 一箱多少件，未知留 NULL
  icon_path    TEXT,                        -- 本地缓存路径，相对 ./assets/icons/
  icon_source  TEXT,                        -- 'wiki_cached' / 'manual' / 'placeholder'
  updated_at   INTEGER NOT NULL
);

-- 物品别名（中英文/历史名 → canonical_id）
CREATE TABLE item_aliases (
  alias        TEXT PRIMARY KEY,            -- 与 ingest 文本完全一致的字符串
  canonical_id TEXT NOT NULL REFERENCES items(canonical_id)
);

-- 仓库（建筑里的具体一格仓）
CREATE TABLE stockpiles (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  war_id        INTEGER NOT NULL REFERENCES wars(id),
  region        TEXT NOT NULL,              -- 例 'Umbral Wildwood'
  sub_region    TEXT NOT NULL,              -- 例 'The Gap'
  building_name TEXT NOT NULL,              -- 例 '海港'
  visibility    TEXT NOT NULL,              -- 'Public' / 'Private'
  player_alias  TEXT NOT NULL,              -- 上传者起的别名，例 '东门仓'
  first_seen_at INTEGER NOT NULL,
  last_seen_at  INTEGER NOT NULL,
  UNIQUE (war_id, region, sub_region, building_name, visibility, player_alias)
);
CREATE INDEX idx_stockpiles_war ON stockpiles(war_id);

-- 快照（每次成功 ingest 写一条）
CREATE TABLE snapshots (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  stockpile_id    INTEGER NOT NULL REFERENCES stockpiles(id),
  war_id          INTEGER NOT NULL,         -- 冗余，便于按 war 清理
  uploader_id     TEXT NOT NULL REFERENCES users(steam64id),
  captured_at     INTEGER NOT NULL,         -- 头行解析出的游戏内时间戳
  uploaded_at     INTEGER NOT NULL,         -- 服务端入库时间
  pos_x           REAL,                     -- 头行 X
  pos_y           REAL,                     -- 头行 Y
  raw_sha256      TEXT NOT NULL,            -- 原始文本哈希，幂等去重
  raw_text        TEXT NOT NULL             -- 原始文本完整保留
);
CREATE INDEX idx_snap_stockpile_time ON snapshots(stockpile_id, captured_at DESC);
CREATE INDEX idx_snap_war ON snapshots(war_id);
CREATE UNIQUE INDEX idx_snap_dedup ON snapshots(stockpile_id, raw_sha256);

-- 快照明细（每物品一行）
CREATE TABLE snapshot_items (
  snapshot_id    INTEGER NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
  canonical_id   TEXT NOT NULL REFERENCES items(canonical_id),
  is_crate       INTEGER NOT NULL,          -- 0=散件, 1=整箱
  quantity       INTEGER NOT NULL,
  PRIMARY KEY (snapshot_id, canonical_id, is_crate)
);
CREATE INDEX idx_snap_items_item ON snapshot_items(canonical_id);

-- 当前态视图（每仓最新一份 snapshot）
CREATE VIEW v_latest_snapshot AS
SELECT s.*
FROM snapshots s
JOIN (
  SELECT stockpile_id, MAX(captured_at) AS m
  FROM snapshots
  GROUP BY stockpile_id
) t ON t.stockpile_id = s.stockpile_id AND t.m = s.captured_at;
```

### 3.2 关键设计取舍说明

- **`raw_text` 全量保留**：磁盘成本可控（单次约 12 KB，单 war 约 50~100 MB），换来"任何时候可重放/重解析/审计"的能力，对严格模式 ingest 至关重要
- **`(stockpile_id, raw_sha256)` 唯一**：相同内容反复粘贴自动去重，不报错
- **`captured_at` 用游戏内时间**：头行的 `2026.06.13-06.48.16` 是导出时游戏内时间，作为"事实时间"；`uploaded_at` 仅用于审计
- **不存"差分表"**：差分 = 相邻两条 snapshot_items 的 SQL diff，按需算，不冗余存
- **`is_crate` 拆两行而不是宽表**：样本中"散件 + 箱装"是分别独立的物品形态，宽表会有大量 NULL，长表更直接

---

## 4. 解析器规约（严格模式）

### 4.1 文件级结构
```
LINE 1   ：HEADER（地区头行）
LINE 2~N ：ITEM 或 BLANK（空行作为段分隔符，不语义化）
```

### 4.2 头行语法
```
HEADER := REGION " - " SUB_REGION " - " BUILDING " - " VISIBILITY " - X: " FLOAT " Y: " FLOAT "," TIMESTAMP
VISIBILITY := "Public" | "Private"
TIMESTAMP := YYYY "." MM "." DD "-" HH "." MM "." SS
```

切分顺序（左→右）：
1. 从右向左找第一个 `,`，右半为 TIMESTAMP（不含中文逗号干扰，时间戳本身无逗号）
2. 在左半中按 `" - "`（带前后空格的横线）切，期望恰好 4 段：`REGION / SUB_REGION / BUILDING / VISIBILITY+坐标`
3. 第 4 段按 `" - X: "` 切：左边是 `Public`/`Private`，右边按 ` Y: ` 切出 X、Y

### 4.3 物品行语法
```
ITEM_LINE := NAME [" (箱装)"] "," INTEGER
```

切分顺序：
1. 从右向左找第一个 `,`，右半 trim 后必须能 parse 为非负整数 → 否则 reject
2. 左半若以 ` (箱装)` 结尾（注意半角空格 + 中文括号 + "箱装"两字）→ `is_crate=1`，剥掉后缀
3. 剩下即为 `name`，与 `item_aliases.alias` 完全字符串相等查找
4. **找不到 alias → 整个文件 reject**，错误信息标明行号 + 该物品名

### 4.4 严格策略与示例错误

| 场景 | 处理 |
|---|---|
| 头行格式不符 | 文件 reject，HTTP 400，message: `"header malformed"` |
| 数量为负或非数字 | reject |
| 任意物品名不在 alias 表 | reject，附该物品名到 message |
| 重复物品行（散件/箱装均同时出现两遍） | 取最后一行（实际样本里没出现，保险措施） |
| 空文件 / 仅头行 | reject |
| 文件中存在游戏更新引入的新物品 | reject，owner 必须先在 `items` + `item_aliases` 加映射 |

### 4.5 物品名归一化注意
- 中文标点 / 英文标点 / 引号样式严格保持原文
- 不做 NFC/NFD 转换、不去空格
- 大小写敏感
- alias 表的种子数据从样本里一次性提取（见 §8.1）

---

## 5. HTTP API

### 5.1 公约
- 所有路径以 `/api/` 开头返回 JSON；非 `/api/` 返回 HTML（HTMX 直出）
- 鉴权：`Cookie: sid=<session_token>`，HttpOnly Secure SameSite=Strict
- 错误体：`{"error": {"code": "string", "message": "string"}}`

### 5.2 端点

| Method | Path | 鉴权 | 用途 |
|---|---|---|---|
| GET  | `/login` | guest | 登录页 |
| POST | `/login` | guest | 提交 steam64id+password，发 session cookie |
| GET  | `/register` | guest | 注册/重置密码页 |
| POST | `/register` | guest | 完成注册或重置 |
| POST | `/logout` | member | 销毁 session |
| GET  | `/` | member | dashboard：当前 War 概览 |
| GET  | `/wars` | member | War 列表与归档浏览入口 |
| GET  | `/wars/:id` | member | 指定 war 的仓库列表 |
| GET  | `/stockpiles/:id` | member | 单仓详情 + 物品折线图 |
| GET  | `/items/:canonical_id` | member | 单物品跨仓汇总 |
| GET  | `/upload` | member | 上传粘贴页 |
| POST | `/api/ingest` | member | 提交一份 .txt 内容 |
| GET  | `/admin` | owner | 管理后台（白名单/War/物品维护） |
| POST | `/api/admin/whitelist` | owner | 添加/移除白名单 |
| POST | `/api/admin/war/new` | owner | 开新 War（旧 active 自动归档） |
| POST | `/api/admin/war/:id/end` | owner | 显式结束 War 但不开新场 |
| POST | `/api/admin/items` | owner | 创建/编辑 item + alias |
| POST | `/api/admin/user/:id/reset` | owner | 触发该用户必须重置密码 |
| POST | `/api/admin/user/:id/kick` | owner | 软删 + 失效所有 session |

### 5.3 `POST /api/ingest` 请求体
```json
{
  "raw_text": "...完整 .txt 文本...",
  "player_alias": "东门仓",
  "is_new_alias": false
}
```

服务端流程：
```
1. 解析 raw_text（严格模式）
2. 取 session.steam64id 作为 uploader
3. 查/建 stockpile by (war_id, region, sub_region, building, visibility, player_alias)
   - 若 is_new_alias=false 但库里没有此别名 → 400，提示前端弹"是否新建别名"
4. 计算 raw_sha256
5. INSERT snapshot（命中 dedup 唯一索引 → 200 但 message='duplicate'）
6. 批量 INSERT snapshot_items
7. 返回 stockpile_id、snapshot_id、解析出的物品数
```

---

## 6. 前端结构（Vue 3 + Element Plus + ECharts）

### 6.1 技术栈定型

| 角色 | 选型 | 备注 |
|---|---|---|
| 框架 | Vue 3（`<script setup>` + Composition API） | 中文文档生态最完整 |
| 构建 | Vite | 开发期 HMR、产物 ES2020 |
| 包管理 | pnpm | 节省磁盘，CI/本地一致 |
| 路由 | Vue Router 4 | 客户端路由，hash 模式避免 nginx 子路径配置 |
| 状态 | Pinia | 仅用于"当前用户"等少量全局态 |
| 组件库 | Element Plus（含 Element Plus Icons） | 中文最强、`<el-table>` 满足长表格 |
| HTTP | axios 或原生 fetch | 都行，倾向 fetch + 小封装 |
| 图表 | ECharts 5（按需引入） | 折线图 / 时间轴 / 数据缩放 |
| 样式 | Element Plus 暗色主题为底，少量自定义 CSS | 不引 Tailwind |
| 国际化 | 默认中文，i18n 留接口不做 | 物品中英文都从 `items` 表来 |

### 6.2 项目结构

```
web/                              # 前端独立子工程
├── package.json
├── vite.config.ts                # 配置 base、proxy（dev 转发 /api 到 :8080）
├── tsconfig.json                 # TypeScript 严格模式
├── index.html
├── src/
│   ├── main.ts                   # 入口，挂载 ElementPlus、router、pinia
│   ├── api/
│   │   ├── client.ts             # fetch 封装（统一处理 401 → 跳登录）
│   │   ├── auth.ts               # 登录/注册/登出
│   │   ├── stockpiles.ts
│   │   ├── items.ts
│   │   ├── wars.ts
│   │   └── admin.ts
│   ├── stores/
│   │   └── user.ts               # Pinia：当前登录用户、is_owner
│   ├── router/
│   │   └── index.ts
│   ├── views/
│   │   ├── LoginView.vue
│   │   ├── RegisterView.vue
│   │   ├── DashboardView.vue     # 当前 War 总览
│   │   ├── UploadView.vue        # 粘贴 .txt 提交
│   │   ├── StockpileDetailView.vue
│   │   ├── ItemDetailView.vue
│   │   ├── WarsView.vue
│   │   └── admin/
│   │       ├── AdminLayout.vue
│   │       ├── WhitelistView.vue
│   │       ├── WarManageView.vue
│   │       └── ItemManageView.vue
│   ├── components/
│   │   ├── StockpileTable.vue
│   │   ├── ItemQuantityCell.vue
│   │   ├── TimelineChart.vue     # ECharts 封装
│   │   └── IconImg.vue           # 带 fallback 的图标组件
│   └── styles/
│       └── theme.css             # Foxhole 风暗色覆盖
└── dist/                         # build 产物，由 Go embed 嵌入二进制
```

### 6.3 前后端对接

```
开发态：
  vite dev (:5173) ──proxy /api──► go run main.go (:8080)
                  └─ /icons proxy ─►

生产态：
  pnpm build → web/dist/
  Go embed.FS 嵌入 web/dist/* → 单二进制
  / 与 /assets/* 由 Go 直接 serve dist 内容
  /api/* 走 Go handler
  /icons/* 走 Go handler
```

### 6.4 关键 UI 组件落点

| 页面 | 主要组件 | 说明 |
|---|---|---|
| Dashboard | `<el-table>` + `<el-tag>` | 列：地区/建筑/别名/最后更新/上传者，行点击进详情 |
| StockpileDetail | `<el-table>` 带分组 + `<el-collapse>` | 按 category 折叠，每物品行展开 ECharts |
| ItemDetail | ECharts 多系列折线（每仓一条） + `<el-table>` 总和 | |
| Upload | `<el-input type="textarea">` + 解析预览表 + `<el-form>` 提交 | |
| Admin/Whitelist | `<el-table>` + `<el-button>` 增删 + `<el-dialog>` 二次确认 | |
| Admin/War | `<el-steps>` 显示 War 状态 + `<el-popconfirm>` 删除 | |
| Admin/Item | `<el-table>` + `<el-input>` 内嵌编辑 + 图标上传 `<el-upload>` | |

### 6.5 图表细节（ECharts）
- 单仓单物品折线：x=时间，y=数量（散件 + 箱装两条系列）
- 单物品跨仓：堆叠折线，每仓一个系列，hover 显示该仓 captured_at 上传者
- 时间轴 `dataZoom` 默认显示最近 24h，可拖拽到全 War
- 暗色主题用 `echarts.registerTheme` 注册一份 Foxhole 配色

### 6.6 工作流约束
- **vibecoding 友好**：90% 业务页用 Element Plus 现成组件拼，自定义 CSS 不超过 50 行
- 所有 API 调用走 `src/api/*.ts`，业务组件不直接 fetch
- `<IconImg>` 组件统一处理图标 fallback，业务模板不写 `onerror`
- 表格分页/排序/筛选交给 Element Plus，后端返回时不做客户端不可见的隐式过滤

---

## 7. 图标资源策略

### 7.1 决策（你的关切）
- **首选**：服务端 lazy cache（owner 在 `items` 编辑页填 wiki URL → 点"抓取"按钮 → 服务器下载到 `./assets/icons/{canonical_id}.png`）
- **未维护图标的物品**：前端 fallback 到 `_unknown.png`
- **Wiki 缺图**：owner 可手动上传 PNG，覆盖该 canonical_id 的本地文件
- **绝不在前端 hotlink**：避免 wiki 改 URL/防盗链/CDN 抖动直接拖垮页面

### 7.2 实现成本
- `./assets/icons/` 单图标 ~5~30 KB，全量 ~500 物品 ≈ 5~10 MB，可接受
- owner 若批量懒得手点，提供 `POST /api/admin/items/refresh-icons` 一次性扫所有缺图 item 去拉

### 7.3 关于 Wiki 是否覆盖最新物品
- 现实：Foxhole Wiki 在大版本（如 Naval Update）后通常 1~2 周内补图，少数冷门物品可能拖到下个版本
- 应对：`items.icon_source` 区分 `wiki_cached` / `manual` / `placeholder`，admin 列表可按 placeholder 过滤一眼看出"哪些还没图"

---

## 8. 物品主数据初始化

### 8.1 种子方法
1. 从 `样本.txt` 抽取所有不同物品名（剥离 ` (箱装)` 后去重）
2. 人工/脚本对照 Wiki 填入 `name_en`、`category`、`faction`、`crate_size`
3. 一次性 INSERT 到 `items` + `item_aliases`（中文名 → canonical_id 自映射）
4. 保存为 `seeds/items_v1.sql`，部署时执行
5. 后续游戏更新引入新物品时，由 owner 在 `/admin` 页加（不走自动学习）

### 8.2 canonical_id 命名约定
```
{category_short}_{name_snake_en}
例：
  rifle_aalto          → 'Aalto风暴步枪24'
  ammo_792             → '7.92mm'
  mat_basic_materials  → '基础材料'
  veh_obrien_freeman   → "O'Brien V.101 Freeman"
```

字符集：`[a-z0-9_]`，统一小写 ASCII。

---

## 9. War 管理与归档

### 9.1 状态机
```
            +-------+   owner: 开新 War
            | (无)  | -------------------+
            +-------+                    |
                                         v
+-----------+   owner: 结束本场 War    +-----------+
| archived  | <-----------------------|  active   |
+-----------+                          +-----------+
      ^                                      |
      | owner: 归档时自动执行                 |
      | （旧 active 在新 War 创建时归档）     |
```

### 9.2 保留策略（你选 keep_all_until_full）
- 默认无自动清理
- `/admin` 页显示 `data/stock.db` 文件大小 + 每 war 的快照数
- VPS 磁盘水位告警留给 owner 自己决定何时删
- 提供"导出 JSON 备份"按钮（按 war 导出整场 snapshots+items），便于线下分析后再决定是否清

### 9.3 删除（owner 显式触发）
```
POST /api/admin/war/:id/delete
  → 校验该 war 不是 active
  → 强制要求传 confirm_text="DELETE WAR <label>"
  → 级联删 stockpiles + snapshots + snapshot_items
  → VACUUM
```

---

## 10. 部署与运行

### 10.1 目录结构（VPS 上）
```
/opt/foxhole-stockmgr/
├── foxhole-stockmgr           # 单二进制
├── config.toml                # owner Steam64ID、监听地址、cookie 密钥
├── data/
│   └── stock.db               # SQLite，WAL
├── assets/
│   ├── icons/                 # 物品图标缓存
│   └── _embedded/             # 由 go embed 提供，运行时不读盘
└── logs/
    └── app.log                # 轮转，保留 7 天
```

### 10.2 systemd unit（`/etc/systemd/system/foxhole-stockmgr.service`）
```ini
[Unit]
Description=Foxhole Stockpile Manager
After=network.target

[Service]
Type=simple
User=foxhole
WorkingDirectory=/opt/foxhole-stockmgr
ExecStart=/opt/foxhole-stockmgr/foxhole-stockmgr -config config.toml
Restart=on-failure
RestartSec=5
LimitNOFILE=4096
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/foxhole-stockmgr/data /opt/foxhole-stockmgr/assets/icons /opt/foxhole-stockmgr/logs

[Install]
WantedBy=multi-user.target
```

### 10.3 Nginx 片段
```nginx
location /foxhole/ {
    proxy_pass http://127.0.0.1:8080/;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-Proto $scheme;
    client_max_body_size 1m;
}
```

### 10.4 资源占用预估
| 项 | 估值 |
|---|---|
| 二进制大小 | 15~20 MB |
| 常驻 RAM | 30~80 MB |
| 单 war 30 天 SQLite | 50~200 MB |
| 图标缓存 | 5~10 MB |
| CPU 平均 | <1% |
| 平均延迟 | ingest <50ms / 列表页 <30ms |

---

## 11. 工作区目录结构（开发期）

> 严格遵循工作区规则：所有产物落在 `J:\Developing\Foxhole_project\`。

```
J:\Developing\Foxhole_project\
├── .cursor\rules\workspace-scope.mdc   # 已存在
├── DESIGN.md                           # 本文件
├── 样本.txt                            # 样本，不动
├── README.md                           # 简短入口（v0 之后补）
├── cmd\
│   └── server\
│       └── main.go                     # 入口
├── internal\
│   ├── parser\                         # 解析器 + 单元测试
│   ├── store\                          # SQLite 访问层
│   ├── auth\                           # session + argon2id
│   ├── ingest\                         # 业务编排
│   ├── icons\                          # 图标抓取 / fallback
│   └── web\                            # HTTP handler + HTMX 模板
├── web\
│   ├── templates\                      # *.html
│   └── static\                         # uPlot.js, htmx.min.js, alpine.min.js, app.css
├── seeds\
│   └── items_v1.sql
├── migrations\
│   └── 0001_init.sql
├── scripts\
│   ├── extract_items_from_sample.py    # 一次性抽样本物品名
│   └── deploy.sh                       # rsync + systemctl restart
├── tmp\                                # 调试输出（gitignore）
├── logs\                               # 本地运行日志（gitignore）
├── build\                              # go build 输出（gitignore）
└── go.mod / go.sum
```

---

## 12. 实施顺序（建议下一步）

按依赖链推进，每完成一项形成可独立验证的产物：

1. **解析器 MVP**：内嵌 `样本.txt` 做 fixture，输出结构化 JSON，纯函数，无 IO
2. **物品种子 SQL**：`scripts/extract_items_from_sample.py` 跑一遍 → `seeds/items_v1.sql`
3. **DB 层 + 迁移**：sqlite3 / pure-go `modernc.org/sqlite`，跑 migration + seeds
4. **认证骨架**：register / login / logout / 中间件
5. **ingest 端到端**：粘贴页 → POST → 解析 → 落库，能看到数据
6. **当前态视图页**：仓列表 + 单仓详情
7. **折线图**：uPlot 接 `/api/stockpiles/:id/timeline`
8. **admin 页**：白名单 + War 管理 + 物品维护
9. **图标管线**：lazy 抓取 + manual 上传
10. **systemd + nginx 部署脚本**

---

## 13. 待办的二次决策（v1 上线前不阻塞）

- 物品 `category` / `faction` 的具体枚举值（先用样本里观察到的段落归纳，可后改）
- War 切换时是否提示"当前还有未关闭的活跃仓需要标记？"
- 多语言：v1 中文为主，英文物品名只作匹配辅助；i18n 留接口不实现

---

## 14. 与工作区规则的对齐

- 所有源码与文档生成于 `J:\Developing\Foxhole_project\` 内
- 不引用 `K:\` 任何路径
- 不引用 `J:\Developing\` 同级其他项目（CCB / Civ6 等）
- 临时调试产物落 `tmp\` / `logs\`，不外溢