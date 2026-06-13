# foxhole-quartermaster

Foxhole 军团仓库管理系统。成员把游戏内导出的仓库 .txt 粘贴到网页 → 服务器解析入库 → 不在游戏里的成员通过浏览器查看军团整体物资、单仓存量、历史折线。

- 后端：Go 1.22 + chi + SQLite (modernc.org/sqlite, 纯 Go) + argon2id
- 前端：Vue 3 + Element Plus + ECharts + Vite (pnpm)
- 部署：单二进制（前端 dist 用 `embed.FS` 嵌入），Caddy/Nginx 反代 + systemd

设计文档见 [`DESIGN.md`](./DESIGN.md)，分阶段落地计划见 [`PLAN_ADDENDUM.md`](./PLAN_ADDENDUM.md)。

## 快速开发

```
# 一次性激活工具链（Windows PowerShell）
. ./scripts/activate.ps1

# 后端
go run ./cmd/server -config config.example.toml

# 前端 dev (另一个终端)
pnpm -C web install
pnpm -C web dev    # http://localhost:5173 ，自动 proxy /api 到 :8080
```

## 生产构建（单二进制）

```
pnpm -C web install --frozen-lockfile
pnpm -C web build
go build -tags embedweb -o build/foxhole-quartermaster ./cmd/server
```

## 工作区约束

所有文件必须落在 `J:\Developing\Foxhole_project\` 内。详见 `.cursor/rules/workspace-scope.mdc`。

## License

MIT