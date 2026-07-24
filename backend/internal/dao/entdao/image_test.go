package entdao

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Lestine-Yan/irisImg/backend/config"
	"github.com/Lestine-Yan/irisImg/backend/ent/apikey"
	"github.com/Lestine-Yan/irisImg/backend/internal/dao"
	"github.com/Lestine-Yan/irisImg/backend/internal/model"
)

// newTestDAO 在临时目录打开一个真实的 SQLite 数据库并完成迁移。
// modernc.org/sqlite 是纯 Go 驱动，无需 CGO，测试可离线运行。
func newTestDAO(t *testing.T) dao.ImageDAO {
	t.Helper()
	cfg := config.DatabaseConfig{
		Driver:      "sqlite",
		DSN:         filepath.Join(t.TempDir(), "test.db"),
		AutoMigrate: true,
	}
	client, err := Open(cfg)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	if err := Migrate(context.Background(), client, cfg); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewImageDAO(client)
}

func sampleImage() *model.Image {
	return &model.Image{
		Filename:   "cat.png",
		StoredPath: "2026/06/cat.png",
		URL:        "/i/2026/06/cat.png",
		Size:       2048,
		MimeType:   "image/png",
		Width:      100,
		Height:     80,
		Hash:       "abc123",
	}
}

func TestImageDAO_CreateAndGet(t *testing.T) {
	d := newTestDAO(t)
	ctx := context.Background()

	created, err := d.Create(ctx, sampleImage())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected non-zero id")
	}
	if created.CreatedAt.IsZero() {
		t.Fatalf("expected created_at to be filled")
	}

	got, err := d.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Hash != "abc123" || got.Filename != "cat.png" {
		t.Fatalf("unexpected row: %+v", got)
	}

	byHash, err := d.GetByHash(ctx, "abc123")
	if err != nil {
		t.Fatalf("get by hash: %v", err)
	}
	if byHash.ID != created.ID {
		t.Fatalf("get by hash id mismatch: %d != %d", byHash.ID, created.ID)
	}
}

func TestImageDAO_NotFound(t *testing.T) {
	d := newTestDAO(t)
	ctx := context.Background()

	if _, err := d.GetByID(ctx, 9999); !errors.Is(err, dao.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if _, err := d.GetByHash(ctx, "missing"); !errors.Is(err, dao.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := d.Delete(ctx, 9999); !errors.Is(err, dao.ErrNotFound) {
		t.Fatalf("expected ErrNotFound on delete, got %v", err)
	}
}

func TestImageDAO_ListAndDelete(t *testing.T) {
	d := newTestDAO(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		img := sampleImage()
		img.Hash = "hash" + string(rune('a'+i))
		img.StoredPath = "p/" + img.Hash
		if _, err := d.Create(ctx, img); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	items, total, err := d.List(ctx, model.ImageListQuery{Limit: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items with limit, got %d", len(items))
	}

	if err := d.Delete(ctx, items[0].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, _, err := d.List(ctx, model.ImageListQuery{}); err != nil {
		t.Fatalf("list after delete: %v", err)
	}
}

// TestImageDAO_ListFilterAndOrder 覆盖 List 的过滤、排序、分页：
//   - 按 key_id 过滤
//   - asc / desc 排序方向
//   - offset/limit 分页与 total 的关系
//
// 为让排序可测，这里直接用 ent client 写入带明确 created_at 的记录
// （dao.Create 走 default now，无法控制时间）。
func TestImageDAO_ListFilterAndOrder(t *testing.T) {
	d := newTestDAO(t)
	ctx := context.Background()
	impl := d.(*imageDAO)

	// image.key_id 有外键约束到 api_keys.id，先插入两把密钥（空表自增得 id=1、2），
	// 否则带 key_id 的 image 落库会触发 FOREIGN KEY constraint failed。
	keyNames := []string{"key-one", "key-two"}
	for _, name := range keyNames {
		if _, err := impl.client.ApiKey.Create().
			SetName(name).
			SetKeyHash("hash-"+name).
			SetPrefix("pfx").
			SetScope(apikey.ScopeReadwrite).
			Save(ctx); err != nil {
			t.Fatalf("create key %s: %v", name, err)
		}
	}

	base := time.Now().Truncate(time.Hour).UTC()
	records := []struct {
		hash string
		key  int
		t    time.Time
	}{
		{"h1", 1, base.Add(3 * time.Minute)},
		{"h2", 1, base.Add(1 * time.Minute)},
		{"h3", 2, base.Add(2 * time.Minute)},
	}
	for _, r := range records {
		if _, err := impl.client.Image.Create().
			SetFilename(r.hash + ".png").
			SetStoredPath("p/" + r.hash).
			SetURL("/i/" + r.hash).
			SetSize(100).
			SetMimeType("image/png").
			SetWidth(10).
			SetHeight(10).
			SetHash(r.hash).
			SetNillableKeyID(&r.key).
			SetCreatedAt(r.t).
			Save(ctx); err != nil {
			t.Fatalf("create %s: %v", r.hash, err)
		}
	}

	// 全部升序：h2(1m) → h3(2m) → h1(3m)
	items, total, err := d.List(ctx, model.ImageListQuery{Order: "asc"})
	if err != nil {
		t.Fatalf("list asc: %v", err)
	}
	if total != 3 || len(items) != 3 {
		t.Fatalf("asc: total=%d len=%d", total, len(items))
	}
	if items[0].Hash != "h2" || items[2].Hash != "h1" {
		t.Fatalf("asc order: %s %s %s", items[0].Hash, items[1].Hash, items[2].Hash)
	}

	// 倒序：h1 → h3 → h2
	items, _, err = d.List(ctx, model.ImageListQuery{Order: "desc"})
	if err != nil {
		t.Fatalf("list desc: %v", err)
	}
	if items[0].Hash != "h1" || items[2].Hash != "h2" {
		t.Fatalf("desc order: %s %s %s", items[0].Hash, items[1].Hash, items[2].Hash)
	}

	// 按 key_id=1 过滤：仅 h2、h1，升序
	key1 := 1
	items, total, err = d.List(ctx, model.ImageListQuery{KeyID: &key1, Order: "asc"})
	if err != nil {
		t.Fatalf("list key1: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("key1: total=%d len=%d", total, len(items))
	}
	if items[0].Hash != "h2" || items[1].Hash != "h1" {
		t.Fatalf("key1 order: %s %s", items[0].Hash, items[1].Hash)
	}

	// 分页：升序 offset=1, limit=1 应命中第二条 h3
	items, total, err = d.List(ctx, model.ImageListQuery{Order: "asc", Offset: 1, Limit: 1})
	if err != nil {
		t.Fatalf("list paged: %v", err)
	}
	if total != 3 || len(items) != 1 {
		t.Fatalf("paged: total=%d len=%d", total, len(items))
	}
	if items[0].Hash != "h3" {
		t.Fatalf("paged item=%s, want h3", items[0].Hash)
	}
}

// TestImageDAO_CountTotalSizeCountByRange 覆盖仪表盘用到的三个聚合方法，
// 重点验证空表 SUM 返回 NULL 时 TotalSize 兜底为 0（不报错）--全项目首个 ent 聚合查询。
func TestImageDAO_CountTotalSizeCountByRange(t *testing.T) {
	d := newTestDAO(t)
	ctx := context.Background()
	impl := d.(*imageDAO)

	// 空表：Count=0、TotalSize=0（SUM 返回 NULL 的兜底，关键）、CountByRange=0。
	if n, err := d.Count(ctx); err != nil || n != 0 {
		t.Fatalf("empty Count = %d err %v, want 0", n, err)
	}
	if n, err := d.TotalSize(ctx); err != nil || n != 0 {
		t.Fatalf("empty TotalSize = %d err %v, want 0 (NULL guard)", n, err)
	}

	// 写入 3 张图：sizes 100/200/300，created_at 分别为今天/昨天/前天（控制时间以测区间）。
	now := time.Now()
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, loc)
	records := []struct {
		hash string
		size int64
		t    time.Time
	}{
		{"h1", 100, today},
		{"h2", 200, today.AddDate(0, 0, -1)},
		{"h3", 300, today.AddDate(0, 0, -2)},
	}
	for _, r := range records {
		if _, err := impl.client.Image.Create().
			SetFilename(r.hash + ".png").
			SetStoredPath("p/" + r.hash).
			SetURL("/i/" + r.hash).
			SetSize(r.size).
			SetMimeType("image/png").
			SetWidth(1).
			SetHeight(1).
			SetHash(r.hash).
			SetCreatedAt(r.t).
			Save(ctx); err != nil {
			t.Fatalf("create %s: %v", r.hash, err)
		}
	}

	if n, err := d.Count(ctx); err != nil || n != 3 {
		t.Fatalf("Count = %d err %v, want 3", n, err)
	}
	if n, err := d.TotalSize(ctx); err != nil || n != 600 {
		t.Fatalf("TotalSize = %d err %v, want 600", n, err)
	}

	// CountByRange：今天左闭右开应只命中 h1。
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.AddDate(0, 0, 1)
	if n, err := d.CountByRange(ctx, dayStart, dayEnd); err != nil || n != 1 {
		t.Fatalf("today CountByRange = %d err %v, want 1", n, err)
	}
	// 近 3 天区间 [today-2, today+1) 命中全部 3 张。
	if n, err := d.CountByRange(ctx, dayStart.AddDate(0, 0, -2), dayEnd); err != nil || n != 3 {
		t.Fatalf("3-day CountByRange = %d err %v, want 3", n, err)
	}
	// 未来区间应为 0。
	if n, err := d.CountByRange(ctx, dayEnd, dayEnd.AddDate(0, 0, 1)); err != nil || n != 0 {
		t.Fatalf("future CountByRange = %d err %v, want 0", n, err)
	}
}

// TestImageDAO_ListAndDeleteByKeyID 覆盖按密钥批量查询与删除：
//   - ListByKeyID 只返回该密钥的图片；
//   - DeleteByKeyID 删除该密钥全部图片并返回计数，不影响其他密钥的图片。
func TestImageDAO_ListAndDeleteByKeyID(t *testing.T) {
	d := newTestDAO(t)
	ctx := context.Background()
	impl := d.(*imageDAO)

	// 先建两把密钥（image.key_id 外键依赖）。
	for _, name := range []string{"k1", "k2"} {
		if _, err := impl.client.ApiKey.Create().
			SetName(name).
			SetKeyHash("hash-" + name).
			SetPrefix("pfx").
			SetScope(apikey.ScopeReadwrite).
			Save(ctx); err != nil {
			t.Fatalf("create key %s: %v", name, err)
		}
	}

	mk := func(hash string, keyID int) {
		k := keyID
		if _, err := impl.client.Image.Create().
			SetFilename(hash + ".png").
			SetStoredPath("p/" + hash).
			SetURL("/i/" + hash).
			SetSize(10).
			SetMimeType("image/png").
			SetWidth(1).
			SetHeight(1).
			SetHash(hash).
			SetNillableKeyID(&k).
			Save(ctx); err != nil {
			t.Fatalf("create %s: %v", hash, err)
		}
	}
	mk("a1", 1)
	mk("a2", 1)
	mk("b1", 2)

	items, err := d.ListByKeyID(ctx, 1)
	if err != nil {
		t.Fatalf("list by key 1: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 images for key 1, got %d", len(items))
	}

	n, err := d.DeleteByKeyID(ctx, 1)
	if err != nil {
		t.Fatalf("delete by key 1: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 removed, got %d", n)
	}

	// key1 图片已清空，key2 仍有一张。
	if left, err := d.ListByKeyID(ctx, 1); err != nil || len(left) != 0 {
		t.Fatalf("expected key1 empty, got %d (err %v)", len(left), err)
	}
	if left, err := d.ListByKeyID(ctx, 2); err != nil || len(left) != 1 {
		t.Fatalf("expected key2 still 1, got %d (err %v)", len(left), err)
	}
}

// TestImageDAO_CountByRangeGrouped 覆盖仪表盘按日按来源聚合：
//   - admin（nil KeyID）与各 key 分组正确；
//   - 区间外的记录被排除；
//   - 空区间返回空切片。
func TestImageDAO_CountByRangeGrouped(t *testing.T) {
	d := newTestDAO(t)
	ctx := context.Background()
	impl := d.(*imageDAO)

	// 两把密钥（image.key_id 外键依赖）。
	for _, name := range []string{"k1", "k2"} {
		if _, err := impl.client.ApiKey.Create().
			SetName(name).
			SetKeyHash("hash-" + name).
			SetPrefix("pfx").
			SetScope(apikey.ScopeReadwrite).
			Save(ctx); err != nil {
			t.Fatalf("create key %s: %v", name, err)
		}
	}

	now := time.Now()
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, loc)
	yesterday := today.AddDate(0, 0, -1)

	mk := func(hash string, keyID *int, ts time.Time) {
		if _, err := impl.client.Image.Create().
			SetFilename(hash + ".png").
			SetStoredPath("p/" + hash).
			SetURL("/i/" + hash).
			SetSize(10).
			SetMimeType("image/png").
			SetWidth(1).
			SetHeight(1).
			SetHash(hash).
			SetNillableKeyID(keyID).
			SetCreatedAt(ts).
			Save(ctx); err != nil {
			t.Fatalf("create %s: %v", hash, err)
		}
	}
	key1, key2 := 1, 2
	mk("a1", nil, today)        // admin 今天
	mk("a2", nil, today)        // admin 今天
	mk("k1a", &key1, today)     // key1 今天
	mk("k2a", &key2, today)     // key2 今天
	mk("a3", nil, yesterday)    // admin 昨天（应排除）
	mk("k1b", &key1, yesterday) // key1 昨天（应排除）

	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.AddDate(0, 0, 1)
	got, err := d.CountByRangeGrouped(ctx, dayStart, dayEnd)
	if err != nil {
		t.Fatalf("CountByRangeGrouped today: %v", err)
	}

	// 期望：admin=2、key1=1、key2=1，共 3 组（用 0 作 admin 哨兵）。
	gotMap := map[int]int{}
	for _, g := range got {
		id := 0
		if g.KeyID != nil {
			id = *g.KeyID
		}
		gotMap[id] = g.Count
	}
	if len(got) != 3 || gotMap[0] != 2 || gotMap[1] != 1 || gotMap[2] != 1 {
		t.Fatalf("today groups = %+v (map=%v), want admin=2 key1=1 key2=1", got, gotMap)
	}

	// 未来区间应返回空。
	if g, err := d.CountByRangeGrouped(ctx, dayEnd, dayEnd.AddDate(0, 0, 1)); err != nil || len(g) != 0 {
		t.Fatalf("future groups = %+v err %v, want empty", g, err)
	}
}
