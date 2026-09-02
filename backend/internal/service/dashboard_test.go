package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Lestine-Yan/irisImg/backend/internal/dao"
	"github.com/Lestine-Yan/irisImg/backend/internal/model"
)

// memAPIKeyDAO 是 dao.APIKeyDAO 的内存实现，仅供仪表盘 service 单测使用。
type memAPIKeyDAO struct {
	keys []*model.APIKey
}

func (m *memAPIKeyDAO) List(_ context.Context) ([]*model.APIKey, error) {
	return m.keys, nil
}

// 以下方法仪表盘测试不会命中，返回 not used 以满足接口。
func (m *memAPIKeyDAO) Create(_ context.Context, _ *model.APIKey) (*model.APIKey, error) {
	return nil, errors.New("not used")
}
func (m *memAPIKeyDAO) GetByHash(_ context.Context, _ string) (*model.APIKey, error) {
	return nil, errors.New("not used")
}
func (m *memAPIKeyDAO) GetByID(_ context.Context, _ int) (*model.APIKey, error) {
	return nil, errors.New("not used")
}
func (m *memAPIKeyDAO) Revoke(_ context.Context, _ int) error              { return errors.New("not used") }
func (m *memAPIKeyDAO) UpdateName(_ context.Context, _ int, _ string) (*model.APIKey, error) {
	return nil, errors.New("not used")
}
func (m *memAPIKeyDAO) ResetKey(_ context.Context, _ int, _, _ string) (*model.APIKey, error) {
	return nil, errors.New("not used")
}
func (m *memAPIKeyDAO) Delete(_ context.Context, _ int) error { return errors.New("not used") }
func (m *memAPIKeyDAO) TouchLastUsed(_ context.Context, _ int, _ time.Time) error {
	return errors.New("not used")
}

var _ dao.APIKeyDAO = (*memAPIKeyDAO)(nil)

// memLogDAO 是 dao.LogDAO 的内存实现，仅供仪表盘 service 单测使用。
type memLogDAO struct {
	count int64
}

func (m *memLogDAO) Count(_ context.Context) (int64, error) { return m.count, nil }

func (m *memLogDAO) Create(_ context.Context, _ *model.Log) (*model.Log, error) {
	return nil, errors.New("not used")
}
func (m *memLogDAO) BatchCreate(_ context.Context, _ []*model.Log) error {
	return errors.New("not used")
}
func (m *memLogDAO) List(_ context.Context, _ model.LogQuery) ([]*model.Log, int, error) {
	return nil, 0, errors.New("not used")
}
func (m *memLogDAO) CountByRange(_ context.Context, _, _ time.Time) (int, error) {
	return 0, errors.New("not used")
}
func (m *memLogDAO) ClearAll(_ context.Context) (int64, error) { return 0, errors.New("not used") }
func (m *memLogDAO) ClearInfoGet(_ context.Context) (int64, error) {
	return 0, errors.New("not used")
}

var _ dao.LogDAO = (*memLogDAO)(nil)

// seedImage 直接往内存 DAO 注入一条图片元信息（绕过 service，便于控制 CreatedAt/Size）。
func seedImage(mem *memImageDAO, id int, hash string, size int64, createdAt time.Time) {
	img := &model.Image{ID: id, Hash: hash, Size: size, CreatedAt: createdAt}
	mem.byID[id] = img
	mem.byHash[hash] = img
	if id > mem.nextID {
		mem.nextID = id
	}
}

func TestDashboardService_Overview(t *testing.T) {
	// 构造「今天 / 昨天 / 前天」三个时间点，用于验证按日趋势。
	now := time.Now()
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, loc)
	yesterday := today.AddDate(0, 0, -1)
	dayBefore := today.AddDate(0, 0, -2)

	mem := newMemDAO()
	seedImage(mem, 1, "h1", 100, today)               // 今天 - admin 直传
	seedImageWithKey(mem, 2, "h2", 200, yesterday, 2) // 昨天 - 经 key2(beta)
	seedImage(mem, 3, "h3", 300, dayBefore)           // 前天 - admin 直传
	seedImageWithKey(mem, 4, "h4", 400, today, 1)     // 今天第二张 - 经 key1(alpha)

	keyDAO := &memAPIKeyDAO{keys: []*model.APIKey{
		{ID: 1, Name: "alpha", Revoked: false},
		{ID: 2, Name: "beta", Revoked: false},
		{ID: 3, Name: "gamma", Revoked: true},
	}}
	logDAO := &memLogDAO{count: 9999}

	svc := NewDashboardService(mem, keyDAO, logDAO)

	got, err := svc.Overview(context.Background(), 30)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}

	// 图片总量与存储大小（100+200+300+400=1000）。
	if got.ImagesTotal != 4 {
		t.Fatalf("images_total = %d, want 4", got.ImagesTotal)
	}
	if got.StorageBytes != 1000 {
		t.Fatalf("storage_bytes = %d, want 1000", got.StorageBytes)
	}

	// APIkey 计数：3 总 / 2 有效 / 1 吊销。
	if got.APIKeysTotal != 3 || got.APIKeysActive != 2 || got.APIKeysRevoked != 1 {
		t.Fatalf("apikey counts = total=%d active=%d revoked=%d, want 3/2/1",
			got.APIKeysTotal, got.APIKeysActive, got.APIKeysRevoked)
	}

	// 日志总量。
	if got.LogsTotal != 9999 {
		t.Fatalf("logs_total = %d, want 9999", got.LogsTotal)
	}

	// 趋势：30 天、升序、最后一天是今天。
	if len(got.RecentUploadTrend) != 30 {
		t.Fatalf("trend len = %d, want 30", len(got.RecentUploadTrend))
	}
	if got.RecentUploadTrend[29].Date != today.Format("2006-01-02") {
		t.Fatalf("last trend day = %q, want %q",
			got.RecentUploadTrend[29].Date, today.Format("2006-01-02"))
	}
	if got.RecentUploadTrend[29].Count != 2 { // 今天两张
		t.Fatalf("today count = %d, want 2", got.RecentUploadTrend[29].Count)
	}
	if got.RecentUploadTrend[28].Count != 1 { // 昨天一张
		t.Fatalf("yesterday count = %d, want 1", got.RecentUploadTrend[28].Count)
	}
	if got.RecentUploadTrend[27].Count != 1 { // 前天一张
		t.Fatalf("day-before count = %d, want 1", got.RecentUploadTrend[27].Count)
	}
	if got.RecentUploadTotal != 4 { // 合计 4
		t.Fatalf("recent_upload_total = %d, want 4", got.RecentUploadTotal)
	}
	if got.Days != 30 {
		t.Fatalf("days = %d, want 30", got.Days)
	}

	// 按来源拆分（tooltip 用）：今天 = admin + alpha（各 1），昨天 = beta，前天 = admin。
	todayKeys := got.RecentUploadTrend[29].Keys
	if len(todayKeys) != 2 || !hasKeyCount(todayKeys, "admin", 1) || !hasKeyCount(todayKeys, "alpha", 1) {
		t.Fatalf("today keys = %+v, want admin=1 and alpha=1", todayKeys)
	}
	yesterdayKeys := got.RecentUploadTrend[28].Keys
	if len(yesterdayKeys) != 1 || !hasKeyCount(yesterdayKeys, "beta", 1) {
		t.Fatalf("yesterday keys = %+v, want beta=1", yesterdayKeys)
	}
	dayBeforeKeys := got.RecentUploadTrend[27].Keys
	if len(dayBeforeKeys) != 1 || !hasKeyCount(dayBeforeKeys, "admin", 1) {
		t.Fatalf("day-before keys = %+v, want admin=1", dayBeforeKeys)
	}
	// 无上传的日子 Keys 为空（序列化时 omitempty 省略）。
	if len(got.RecentUploadTrend[0].Keys) != 0 {
		t.Fatalf("earliest day keys = %+v, want empty", got.RecentUploadTrend[0].Keys)
	}
}

// seedImageWithKey 同 seedImage，但关联指定密钥 ID（模拟经 API Key 上传）。
func seedImageWithKey(mem *memImageDAO, id int, hash string, size int64, createdAt time.Time, keyID int) {
	kid := keyID
	img := &model.Image{ID: id, Hash: hash, Size: size, CreatedAt: createdAt, KeyID: &kid}
	mem.byID[id] = img
	mem.byHash[hash] = img
	if id > mem.nextID {
		mem.nextID = id
	}
}

// hasKeyCount 报告 ks 是否包含指定 {name, count} 项（不依赖顺序，用于断言降序排序后的成员）。
func hasKeyCount(ks []model.KeyCount, name string, count int) bool {
	for _, k := range ks {
		if k.Name == name && k.Count == count {
			return true
		}
	}
	return false
}

func TestDashboardService_DefaultDays(t *testing.T) {
	mem := newMemDAO()
	svc := NewDashboardService(mem, &memAPIKeyDAO{}, &memLogDAO{})
	got, err := svc.Overview(context.Background(), 0)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if got.Days != 30 || len(got.RecentUploadTrend) != 30 {
		t.Fatalf("days<=0 should default to 30; got days=%d len=%d", got.Days, len(got.RecentUploadTrend))
	}
}
