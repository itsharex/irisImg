package entdao

import (
	"context"
	"time"

	"github.com/Lestine-Yan/irisImg/backend/ent"
	"github.com/Lestine-Yan/irisImg/backend/ent/image"
	"github.com/Lestine-Yan/irisImg/backend/internal/dao"
	"github.com/Lestine-Yan/irisImg/backend/internal/model"
)

// imageDAO 是 dao.ImageDAO 的 Ent 实现。
type imageDAO struct {
	client *ent.Client
}

// NewImageDAO 基于 Ent 客户端构造 dao.ImageDAO。
func NewImageDAO(client *ent.Client) dao.ImageDAO {
	return &imageDAO{client: client}
}

// 编译期断言：imageDAO 必须实现 dao.ImageDAO。
var _ dao.ImageDAO = (*imageDAO)(nil)

func (d *imageDAO) Create(ctx context.Context, img *model.Image) (*model.Image, error) {
	row, err := d.client.Image.Create().
		SetFilename(img.Filename).
		SetStoredPath(img.StoredPath).
		SetURL(img.URL).
		SetSize(img.Size).
		SetMimeType(img.MimeType).
		SetWidth(img.Width).
		SetHeight(img.Height).
		SetHash(img.Hash).
		SetNillableKeyID(img.KeyID).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return toModel(row), nil
}

func (d *imageDAO) GetByID(ctx context.Context, id int) (*model.Image, error) {
	row, err := d.client.Image.Get(ctx, id)
	if err != nil {
		return nil, wrapErr(err)
	}
	return toModel(row), nil
}

func (d *imageDAO) GetByHash(ctx context.Context, hash string) (*model.Image, error) {
	row, err := d.client.Image.Query().
		Where(image.HashEQ(hash)).
		First(ctx)
	if err != nil {
		return nil, wrapErr(err)
	}
	return toModel(row), nil
}

func (d *imageDAO) List(ctx context.Context, q model.ImageListQuery) ([]*model.Image, int, error) {
	total, err := d.countImages(ctx, q)
	if err != nil {
		return nil, 0, err
	}

	query := d.client.Image.Query()
	if q.KeyID != nil {
		query = query.Where(image.KeyIDEQ(*q.KeyID))
	}

	// 默认升序；仅当显式指定 "desc" 时才倒序。空字符串按升序处理，契合内容中心需求。
	if q.Order == "desc" {
		query = query.Order(ent.Desc(image.FieldCreatedAt))
	} else {
		query = query.Order(ent.Asc(image.FieldCreatedAt))
	}
	if q.Offset > 0 {
		query = query.Offset(q.Offset)
	}
	if q.Limit > 0 {
		query = query.Limit(q.Limit)
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, 0, err
	}

	items := make([]*model.Image, 0, len(rows))
	for _, row := range rows {
		items = append(items, toModel(row))
	}
	return items, total, nil
}

// countImages 统计符合过滤条件的图片总数，过滤条件与 List 保持一致。
func (d *imageDAO) countImages(ctx context.Context, q model.ImageListQuery) (int, error) {
	query := d.client.Image.Query()
	if q.KeyID != nil {
		query = query.Where(image.KeyIDEQ(*q.KeyID))
	}
	return query.Count(ctx)
}

// Count 返回图片总量（无过滤）。
func (d *imageDAO) Count(ctx context.Context) (int64, error) {
	n, err := d.client.Image.Query().Count(ctx)
	if err != nil {
		return 0, err
	}
	return int64(n), nil
}

// TotalSize 返回全部图片 size 字段之和（字节）。
//
// 用 ent.Sum 聚合一条 SQL 完成。ent 的 Scan 底层是 sql.ScanSlice，只接受 slice 目标，
// 故扫描到 []struct{ Total *int64 }；空表时 SQL SUM 返回 NULL（单行单列），*int64 接收为 nil，
// 兜底返回 0。ent.As 别名 total 配合 struct 的 sql tag 精确映射列名。
// 用 *int64 直接承接 NULL 而非「先 Count 判空」，避免 Count 与 SUM 之间的 TOCTOU 窗口
// （并发清空图片表会使 SUM 返回 NULL，[]int64 无法承接 NULL 而报错 500），
// 也省去一次冗余 Count 往返。全项目首个聚合查询。
func (d *imageDAO) TotalSize(ctx context.Context) (int64, error) {
	var v []struct {
		Total *int64 `sql:"total"`
	}
	if err := d.client.Image.Query().
		Aggregate(ent.As(ent.Sum(image.FieldSize), "total")).
		Scan(ctx, &v); err != nil {
		return 0, err
	}
	if len(v) == 0 || v[0].Total == nil {
		return 0, nil // 空表 SUM 返回 NULL
	}
	return *v[0].Total, nil
}

// CountByRange 统计 [start, end) 时间区间（按 created_at）新增的图片数，供仪表盘按日聚合。
//
// 时区对齐：modernc.org/sqlite 把 time.Time 按 t.String() 文本绑定、SQLite 按字节序比较，
// 存储用 time.Now()（本地时区），查询参数须用同一时区偏移才能保证字节序与时刻序一致。
// 此处照搬 entdao/log.go:buildLogPreds 的 .In(time.Local) 写法。
func (d *imageDAO) CountByRange(ctx context.Context, start, end time.Time) (int64, error) {
	n, err := d.client.Image.Query().
		Where(image.CreatedAtGTE(start.In(time.Local)), image.CreatedAtLT(end.In(time.Local))).
		Count(ctx)
	if err != nil {
		return 0, err
	}
	return int64(n), nil
}

// CountByRangeGrouped 统计 [start, end) 时间区间（按 created_at）新增图片数，按 key_id 分组返回，
// 供仪表盘按日按来源聚合（tooltip 展示「当日每个 key 上传数」）。
//
// 与 CountByRange 同构的时区对齐谓词；用 Select(image.FieldKeyID) 只投影 KeyID 字段，
// 内存按 KeyID 分组——避免 Ent 对可空字段 GroupBy 时 NULL 分组值的不确定性
// （GroupBy 返回的 Group 对 NULL 的表达依驱动而异，直接读 *int 更稳妥）。
// key_id 自增从 1 开始，用 0 作 admin 哨兵（nil KeyID -> 0），与真实密钥 ID 无冲突。
// 返回值未解析名称，由 service 据 apiKeyDAO.List 解析为 model.KeyCount。
func (d *imageDAO) CountByRangeGrouped(ctx context.Context, start, end time.Time) ([]model.KeyGroupCount, error) {
	rows, err := d.client.Image.Query().
		Where(image.CreatedAtGTE(start.In(time.Local)), image.CreatedAtLT(end.In(time.Local))).
		Select(image.FieldKeyID).
		All(ctx)
	if err != nil {
		return nil, err
	}
	// keyID（0 = admin 哨兵）-> 当日计数
	groups := make(map[int]int)
	for _, r := range rows {
		id := 0 // admin 哨兵
		if r.KeyID != nil {
			id = *r.KeyID
		}
		groups[id]++
	}
	out := make([]model.KeyGroupCount, 0, len(groups))
	for id, c := range groups {
		var kid *int
		if id != 0 {
			v := id
			kid = &v
		}
		out = append(out, model.KeyGroupCount{KeyID: kid, Count: c})
	}
	return out, nil
}

func (d *imageDAO) Delete(ctx context.Context, id int) error {
	if err := d.client.Image.DeleteOneID(id).Exec(ctx); err != nil {
		return wrapErr(err)
	}
	return nil
}

// ListByIDs 按主键集合批量查询现存的图片记录（不存在的 ID 静默跳过），
// 供批量删除前取 StoredPath 做物理文件清理。
// 空切片防御：直接返回空 slice 不发起查询（ent 对空 variadic IN 的行为依版本而定，显式防御更稳）。
func (d *imageDAO) ListByIDs(ctx context.Context, ids []int) ([]*model.Image, error) {
	if len(ids) == 0 {
		return []*model.Image{}, nil
	}
	rows, err := d.client.Image.Query().
		Where(image.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]*model.Image, 0, len(rows))
	for _, row := range rows {
		items = append(items, toModel(row))
	}
	return items, nil
}

// DeleteByIDs 按主键集合批量删除图片记录，返回实际删除条数（不存在的 ID 静默跳过）。
func (d *imageDAO) DeleteByIDs(ctx context.Context, ids []int) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	n, err := d.client.Image.Delete().
		Where(image.IDIn(ids...)).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// ListByKeyID 返回指定密钥关联的全部图片（不分页），供删除密钥时级联清理使用。
func (d *imageDAO) ListByKeyID(ctx context.Context, keyID int) ([]*model.Image, error) {
	rows, err := d.client.Image.Query().
		Where(image.KeyIDEQ(keyID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]*model.Image, 0, len(rows))
	for _, row := range rows {
		items = append(items, toModel(row))
	}
	return items, nil
}

// DeleteByKeyID 批量删除指定密钥关联的全部图片记录，返回实际删除条数。
func (d *imageDAO) DeleteByKeyID(ctx context.Context, keyID int) (int, error) {
	n, err := d.client.Image.Delete().
		Where(image.KeyIDEQ(keyID)).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// wrapErr 将 Ent 的「记录不存在」错误统一转换为 dao.ErrNotFound。
func wrapErr(err error) error {
	if ent.IsNotFound(err) {
		return dao.ErrNotFound
	}
	return err
}

// toModel 将 Ent 实体转换为跨层的 model.Image。
func toModel(e *ent.Image) *model.Image {
	if e == nil {
		return nil
	}
	return &model.Image{
		ID:         e.ID,
		Filename:   e.Filename,
		StoredPath: e.StoredPath,
		URL:        e.URL,
		Size:       e.Size,
		MimeType:   e.MimeType,
		Width:      e.Width,
		Height:     e.Height,
		Hash:       e.Hash,
		CreatedAt:  e.CreatedAt,
		KeyID:      e.KeyID,
	}
}
