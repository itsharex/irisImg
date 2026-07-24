# internal/dao/entdao/image.go

[`dao.ImageDAO`](../dao.md) 的 Ent 实现。

## 类型

- `imageDAO`：持有 `*ent.Client` 的非导出实现类型。
- `NewImageDAO(client *ent.Client) dao.ImageDAO`：构造函数。
- 编译期断言 `var _ dao.ImageDAO = (*imageDAO)(nil)` 保证接口一致。

## 方法

逐一实现 `ImageDAO` 接口：`Create` / `GetByID` / `GetByHash` / `List` / `ListByKeyID` / `Delete` / `DeleteByKeyID` / `Count` / `TotalSize` / `CountByRange` / `CountByRangeGrouped`。

- `Create`：`SetNillableKeyID(img.KeyID)` 写入可空外键 `key_id`（记录图片由哪把 API 密钥添加；JWT 上传时为 nil 则不设置）。
- `List(ctx, q model.ImageListQuery)`：按 `q.KeyID` 过滤（非 nil 时用 `image.KeyIDEQ`）、按 `q.Order` 排序（`"desc"` 倒序，否则升序）、`q.Offset`/`q.Limit` 为正才生效；总数与过滤条件一致，由私有 `countImages` 统计。
- `ListByKeyID(ctx, keyID)`：`Query().Where(image.KeyIDEQ(keyID)).All(ctx)`，不分页，供删除密钥级联清理使用。
- `DeleteByKeyID(ctx, keyID)`：`Delete().Where(image.KeyIDEQ(keyID)).Exec(ctx)`，返回删除条数。
- `Count(ctx)`：`Image.Query().Count(ctx)` 返回图片总量，`int` -> `int64`，供仪表盘统计。
- `TotalSize(ctx)`：`Aggregate(ent.As(ent.Sum(image.FieldSize), "total")).Scan(ctx, &v)`，`v` 为 `[]struct{ Total *int64 \`sql:"total"\` }`（ent 的 Scan 底层是 `sql.ScanSlice`，只接受 slice 目标，故用 slice 而非单个 struct）。空表时 SQL `SUM` 返回 NULL，`*int64` 接收为 nil，兜底返回 0。用 `*int64` 直接承接 NULL 而非「先 Count 判空」，规避 Count 与 SUM 间的 TOCTOU 窗口（并发清空表会使 SUM 返回 NULL，`[]int64` 无法承接而报错 500）并省去一次冗余 Count 往返。全项目首个聚合查询。
- `CountByRange(ctx, start, end)`：`Where(image.CreatedAtGTE(start.In(time.Local)), image.CreatedAtLT(end.In(time.Local))).Count(ctx)`，按 `created_at` 统计 `[start, end)` 新增图片数，供仪表盘按日聚合。时区对齐照搬 [`log.go`](log.md) 的 `buildLogPreds` 写法（见 [`LOG.md`](../../../LOG.md) 时区陷阱）。
- `CountByRangeGrouped(ctx, start, end)`：与 `CountByRange` 同构的时区对齐谓词，但用 `Select(image.FieldKeyID)` 只投影 `KeyID` 字段后**内存按 `KeyID` 分组**，返回 `[]model.KeyGroupCount`。内存分组而非 ent `GroupBy` 是为规避可空字段 `GroupBy` 时 NULL 分组值依驱动而异的不确定性（直接读 `*int` 更稳妥）；`key_id` 自增从 1 开始，用 0 作 admin 哨兵（`nil` KeyID -> 0），与真实密钥 ID 无冲突。返回值未解析名称，由 service 解析为 `model.KeyCount`。
- 查询单条用 ent 生成的 `image.HashEQ` 等谓词。

## 错误与转换

- `wrapErr`：`ent.IsNotFound(err)` → [`dao.ErrNotFound`](../errors.md)，屏蔽 Ent 错误类型。本函数同时被同包 [`apikey.go`](apikey.md) 复用。
- `toModel(*ent.Image) *model.Image`：把 Ent 实体转换为跨层的 [`model.Image`](../../model/image.md)，并回填 `KeyID`，使 service / api 不直接依赖 Ent。

## 测试

`image_test.go` 在 `t.TempDir()` 打开真实 SQLite（纯 Go 驱动、离线、无 CGO），覆盖创建/查询、未找到、列表分页与删除；`TestImageDAO_ListFilterAndOrder` 额外覆盖按 `key_id` 过滤、asc/desc 排序、offset/limit 分页（用 ent client 写入带明确 `created_at` 的记录以稳定排序，并预置 `api_key` 行满足外键约束）；`TestImageDAO_ListAndDeleteByKeyID` 覆盖 `ListByKeyID` 只返回指定密钥图片、`DeleteByKeyID` 批量删除且不影响其它密钥的图片；`TestImageDAO_CountByRangeGrouped` 覆盖按 `key_id` 分组（含 `nil` KeyID=admin）、区间外记录排除、空区间返回空切片。
