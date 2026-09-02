package service

import (
	"context"
	"testing"
	"time"

	"github.com/Lestine-Yan/irisImg/backend/internal/dao"
	"github.com/Lestine-Yan/irisImg/backend/internal/model"
	"github.com/Lestine-Yan/irisImg/backend/internal/pkg/logger"
)

// mockLogDAO 是 dao.LogDAO 的可控测试替身。
type mockLogDAO struct {
	batched        [][]*model.Log
	countByRng     func(start, end time.Time) int
	cleared        int64
	clearCalled    bool
	clearedGet     int64
	clearGetCalled bool
	total          int64 // Count 返回值，供仪表盘等需要日志总量的场景
}

func (m *mockLogDAO) Create(_ context.Context, l *model.Log) (*model.Log, error) {
	return l, nil
}

func (m *mockLogDAO) BatchCreate(_ context.Context, logs []*model.Log) error {
	// 拷贝一份再存：flush 会用 batch[:0] 复用底层数组，按引用存储会被后一批覆盖。
	cp := make([]*model.Log, len(logs))
	copy(cp, logs)
	m.batched = append(m.batched, cp)
	return nil
}

func (m *mockLogDAO) List(_ context.Context, _ model.LogQuery) ([]*model.Log, int, error) {
	return nil, 0, nil
}

func (m *mockLogDAO) CountByRange(_ context.Context, start, end time.Time) (int, error) {
	if m.countByRng != nil {
		return m.countByRng(start, end), nil
	}
	return 0, nil
}

func (m *mockLogDAO) Count(_ context.Context) (int64, error) {
	return m.total, nil
}

func (m *mockLogDAO) ClearAll(_ context.Context) (int64, error) {
	m.clearCalled = true
	return m.cleared, nil
}

func (m *mockLogDAO) ClearInfoGet(_ context.Context) (int64, error) {
	m.clearGetCalled = true
	return m.clearedGet, nil
}

var _ dao.LogDAO = (*mockLogDAO)(nil)

// flushedTotal 统计所有批量写入的日志条数。
func (m *mockLogDAO) flushedTotal() int {
	n := 0
	for _, b := range m.batched {
		n += len(b)
	}
	return n
}

func TestLogService_RecordFlushesOnClose(t *testing.T) {
	md := &mockLogDAO{}
	s := NewLogService(md, logger.NewNop())

	for i := 0; i < 5; i++ {
		s.Record(&model.Log{Event: model.EventHTTPRequest, Level: model.LevelInfo})
	}
	s.Close()

	if md.flushedTotal() != 5 {
		t.Fatalf("flushed=%d want 5", md.flushedTotal())
	}
}

func TestLogService_Histogram(t *testing.T) {
	md := &mockLogDAO{countByRng: func(start, _ time.Time) int {
		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		if start.Equal(today) {
			return 3
		}
		return 1
	}}
	s := NewLogService(md, logger.NewNop())

	buckets, total, err := s.Histogram(context.Background(), 14)
	s.Close()
	if err != nil {
		t.Fatalf("histogram: %v", err)
	}
	if len(buckets) != 14 {
		t.Fatalf("buckets=%d want 14", len(buckets))
	}
	// 13 天 * 1 + 今天 * 3 = 16
	if total != 16 {
		t.Fatalf("total=%d want 16", total)
	}
	if buckets[13].Count != 3 {
		t.Fatalf("today count=%d want 3", buckets[13].Count)
	}
}

func TestLogService_ClearAllRecordsEvent(t *testing.T) {
	md := &mockLogDAO{cleared: 42}
	s := NewLogService(md, logger.NewNop())

	n, err := s.ClearAll(context.Background(), model.LogContext{Username: "admin"})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if n != 42 {
		t.Fatalf("deleted=%d want 42", n)
	}
	if !md.clearCalled {
		t.Fatalf("ClearAll not called on dao")
	}
	s.Close() // flush 异步缓冲，含 log.clear 审计事件

	found := false
	for _, b := range md.batched {
		for _, l := range b {
			if l.Event == model.EventLogClear && l.Username == "admin" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("log.clear event not recorded")
	}
}

// TestLogService_ClearInfoGetRecordsEvent 验证 ClearInfoGet 只调 dao 的条件删除，
// 并在删除后补记 log.clear_get 审计事件（而非全量清理的 log.clear）。
func TestLogService_ClearInfoGetRecordsEvent(t *testing.T) {
	md := &mockLogDAO{clearedGet: 7}
	s := NewLogService(md, logger.NewNop())

	n, err := s.ClearInfoGet(context.Background(), model.LogContext{Username: "admin"})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if n != 7 {
		t.Fatalf("deleted=%d want 7", n)
	}
	if !md.clearGetCalled {
		t.Fatalf("ClearInfoGet not called on dao")
	}
	if md.clearCalled {
		t.Fatalf("ClearAll should not be called for scope=get")
	}
	s.Close() // flush 异步缓冲，含 log.clear_get 审计事件

	found := false
	for _, b := range md.batched {
		for _, l := range b {
			if l.Event == model.EventLogClearGet && l.Username == "admin" {
				found = true
			}
			if l.Event == model.EventLogClear {
				t.Fatalf("unexpected log.clear event for scope=get")
			}
		}
	}
	if !found {
		t.Fatalf("log.clear_get event not recorded")
	}
}

// TestLogService_RecordAfterCloseNoPanic 验证 Close 之后再调用 Record 不会 panic
// （done 通道保护，buf 永不关闭），且记录被丢弃不落库。
func TestLogService_RecordAfterCloseNoPanic(t *testing.T) {
	md := &mockLogDAO{}
	s := NewLogService(md, logger.NewNop())
	s.Close()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Record after Close panicked: %v", r)
		}
	}()
	s.Record(&model.Log{Event: model.EventHTTPRequest, Level: model.LevelInfo})

	if md.flushedTotal() != 0 {
		t.Fatalf("expected 0 flushed after close, got %d", md.flushedTotal())
	}
}

// TestLogService_ClearAllFlushesPending 验证 ClearAll 先 flush 在途日志再删除：
// 清空后缓冲中的旧日志不应重新落库。
func TestLogService_ClearAllFlushesPending(t *testing.T) {
	md := &mockLogDAO{cleared: 0}
	s := NewLogService(md, logger.NewNop())

	// 入队一条在途访问日志（尚未 flush），随后清空。
	s.Record(&model.Log{Event: model.EventHTTPRequest, Level: model.LevelInfo, Method: "GET"})
	n, err := s.ClearAll(context.Background(), model.LogContext{Username: "admin"})
	s.Close()
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	_ = n

	// flushSync 应在 ClearAll 删除前把在途日志 flush 落库（随后被 ClearAll 删除）；
	// 清空后只应剩 log.clear 审计事件，不应有 http.request 残留。
	httpCount := 0
	clearCount := 0
	for _, b := range md.batched {
		for _, l := range b {
			switch l.Event {
			case model.EventHTTPRequest:
				httpCount++
			case model.EventLogClear:
				clearCount++
			}
		}
	}
	if httpCount != 1 {
		t.Fatalf("in-flight http.request should have been flushed once before clear, got %d", httpCount)
	}
	if clearCount != 1 {
		t.Fatalf("log.clear audit event missing, got %d", clearCount)
	}
}
