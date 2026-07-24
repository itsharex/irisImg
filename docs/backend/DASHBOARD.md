# 仪表盘说明（irisImg）

> 仪表盘是后台默认着陆页（`/dashboard`），用一张只读聚合接口一次性返回图片总量、存储占用、APIkey 计数、日志总量与近 N 天上传趋势，前端据此渲染统计卡片、趋势直方图与四个中心的快捷入口。
> 本文档跨文件讲清楚「指标从哪来」「为什么这么算」「前后端怎么联动」；逐文件文档见各自目录的 `.md`。

---

## 1. 整体设计

- **单接口聚合**：[`GET /admin/dashboard`](./internal/api/dashboard.md) 一次性返回 [`model.DashboardOverview`](./internal/model/dashboard.md)，避免前端发 4-5 个请求拼装。遵循现有 `api -> service -> dao -> model` 分层。
- **只读、无副作用**：仪表盘不写数据、不记业务事件，因此无需 HTTPSOnly 与密码二次确认，仅挂 JWT 受保护组（与 `/system/config` 同级）。
- **复用日志直方图组件**：近 N 天上传趋势复用前端 [`LogsHistogram`](../frontend/components/logs/LogsHistogram.md)（纯 SVG，零第三方依赖），后端趋势单元用 [`model.DashboardTrendDay`](./internal/model/dashboard.md)（在 `DailyCount` 基础上追加按来源拆分的 `Keys`），结构与日志直方图 buckets 兼容；hover 浮动卡据此展示「当日每个 key 上传数」，后台直传显示 admin。

参与的代码文件：

| 角色 | 文件 |
| --- | --- |
| 控制器 | `internal/api/dashboard.go` |
| 业务逻辑 | `internal/service/dashboard.go` |
| DTO | `internal/model/dashboard.go` |
| DAO 接口扩展 | `internal/dao/dao.go`（`ImageDAO.Count/TotalSize/CountByRange/CountByRangeGrouped`、`LogDAO.Count`） |
| DAO 实现 | `internal/dao/entdao/image.go`、`internal/dao/entdao/log.go` |
| 路由装配 | `internal/router/router.go` |
| 前端页面 | `frontend/app/pages/dashboard.vue` |
| 前端 composable | `frontend/app/composables/useDashboard.ts` |
| 前端组件 | `frontend/app/components/dashboard/StatCard.vue`、`frontend/app/components/logs/LogsHistogram.vue`（复用） |

## 2. 接口

| 方法 | 路径 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/v1/admin/dashboard?days=30` | JWT | 返回聚合统计；`days` 默认 30、上限 90，非法值回退 30 |

响应 `data`：

```jsonc
{
  "images_total": 128,
  "storage_bytes": 52428800,        // DB SUM(size)，字节
  "apikeys_total": 3,
  "apikeys_active": 2,
  "apikeys_revoked": 1,
  "logs_total": 9999,
  "recent_upload_trend": [           // 近 30 天，升序、缺日补零；keys 为按来源拆分（无上传则省略）
    { "date": "2026-06-17", "count": 0 },
    // ...
    { "date": "2026-07-16", "count": 5, "keys": [ { "name": "alpha", "count": 3 }, { "name": "admin", "count": 2 } ] }
  ],
  "recent_upload_total": 42,
  "days": 30
}
```

## 3. 指标口径

| 指标 | 来源 | 口径 |
| --- | --- | --- |
| 图片总量 | `ImageDAO.Count` | 无过滤全表 count |
| 存储占用 | `ImageDAO.TotalSize` | DB `SUM(size)`，空表兜底 0（见下「空表 NULL」） |
| APIkey 数 | `APIKeyDAO.List` 内存分桶 | 总数 / 有效（未吊销）/ 已吊销；已删除为物理删除不可统计 |
| 日志总量 | `LogDAO.Count` | 全表 count |
| 近 N 天新增趋势 | `ImageDAO.CountByRangeGrouped` 按日循环 | 以 `Image.CreatedAt` 过滤，左闭右开，缺日补零；每日按 `key_id` 分组并解析标签（后台直传为 admin），供 tooltip 按来源拆分 |

### 为什么存储大小用 DB SUM 而非文件系统遍历

`images.size` 字段已是单一事实来源，`ent.Sum` 一条 SQL 快速准确；`filepath.Walk(rootDir)` 慢、含孤儿文件、与元数据可能不一致，仅适合作可选的「磁盘实际占用」辅助指标（未来增强）。

### 为什么不用 `image.upload` 日志事件数代表新增图片

秒传（同 hash 已存在、未新建记录）也会记一次 `image.upload` 事件，导致重复计数；且日志可被 ClearAll 清空不持久。近 N 天新增必须以 `Image.CreatedAt` 为准。

## 4. 请求链路

```
前端 pages/dashboard.vue
  └─ useDashboard().overview(30)
       └─ useApi.get('/admin/dashboard?days=30')  (自动带 JWT、解包 data)
            └─ api.DashboardAPI.Overview
                 └─ service.DashboardService.Overview(ctx, 30)
                      ├─ imageDAO.Count / TotalSize
                      ├─ apiKeyDAO.List -> 内存按 Revoked 分桶 + 构建 id->name 映射
                      ├─ logDAO.Count
                      └─ uploadTrend(30, nameByID) -> 循环 imageDAO.CountByRangeGrouped(按日按 key)
```

## 5. 前端结构

- **统计卡片区**：4 张 `DashboardStatCard`（图片总量 / 存储占用 / APIkey 数 / 日志总量），`grid-cols-2 lg:grid-cols-4`；加载态用 `animate-pulse` 骨架卡，错误态 rose 虚框 + 重试。
- **趋势图卡片**：白底圆角卡，header 展示「近 N 天新增图片」与合计数，下方复用 `LogsHistogram`（传 `empty-text` / `title-text` 适配图片场景）。
- **快捷入口**：4 张 `NuxtLink` 卡片链接内容中心 / 日志中心 / APIkey 管理 / 系统配置，图标复用 `AppSidebar` 的 heroicons path。
- 配色沿用 iris 色系（`iris-dark` / `iris-violet` / `iris-gold`），危险态统一 `rose-*`。

## 6. 安全与排错

- **时区对齐（高）**：`ImageDAO.CountByRange` 谓词用 `start.In(time.Local)` / `end.In(time.Local)` 对齐服务器本地时区。modernc 驱动按 `t.String()` 文本绑定 `time.Time`、SQLite 按字节序比较，存储用 `time.Now()`（本地时区），查询参数须同一时区偏移才能保证字节序与时刻序一致（与日志链路同一陷阱，见 [`LOG.md`](./LOG.md)）。
- **空表 SUM 返回 NULL（中）**：`TotalSize` 用 `ent.Sum` + `ent.As` 别名 `total`，扫描到 `[]struct{ Total *int64 \`sql:"total"\` }`，`*int64` 直接承接空表 SUM 返回的 NULL（兜底返回 0）。ent 的 `sql.ScanSlice` 只接受 slice 目标、且 NULL 不能写入 `int64`，故用 slice of struct + `*int64` 字段（而非单个 struct 或 `[]int64`）。这样无需「先 Count 判空」，规避 Count 与 SUM 之间的 TOCTOU 500 路径（并发清空图片表会使 SUM 返回 NULL）。已由 `entdao` 真实 SQLite 测试覆盖（含空表 NULL 用例）。
- **聚合性能（低）**：`Overview` 含 30 天循环 `CountByRangeGrouped`（30 次 `Select(key_id)` + 内存分组查询）。SQLite 本地首版足够快；数据量增大后可对结果加 30-60s 内存缓存，或改为单条 `GROUP BY date(created_at), key_id` 聚合。
- **APIkey 已删除不可统计（低）**：`Delete` 为物理删除，UI 仅区分有效 / 已吊销并在副标题标口径。

## 7. 示例

```bash
# 后台登录拿 JWT（见 AUTH.md）
TOKEN="eyJ..."

# 仪表盘聚合统计（近 30 天）
curl -G http://localhost:8080/api/v1/admin/dashboard \
     -H "Authorization: Bearer $TOKEN" \
     --data-urlencode "days=30"
# -> {"code":0,...,"data":{...}}
```

## 8. 扩展与修改建议

- **环境信息只读卡**：复用 `GET /system/config` 展示 storage.root_dir / max_upload_size_mb / allowed_mime_types / 限流 / db driver。
- **存储占用双指标**：主指标 DB SUM「已登记总大小」+ 可选 `filepath.Walk`「磁盘实际占用」（标注含孤儿文件）。
- **MIME 类型分布**：`ent.GroupBy(image.FieldMimeType)` + count，iris 配色药丸展示。
- **APIkey 活跃度**：基于 `LastUsedAt` 标注近 7/30 天活跃密钥数。
- **缓存**：数据量增大后对 `Overview` 加 30-60s 内存缓存。
