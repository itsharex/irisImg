package entdao

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Lestine-Yan/irisImg/backend/config"
	"github.com/Lestine-Yan/irisImg/backend/internal/dao"
	"github.com/Lestine-Yan/irisImg/backend/internal/model"
)

// newTestLogDAO 在临时目录打开真实 SQLite 并完成迁移，返回 LogDAO。
func newTestLogDAO(t *testing.T) dao.LogDAO {
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
	return NewLogDAO(client)
}

func TestLogDAO_CreateAndList(t *testing.T) {
	d := newTestLogDAO(t)
	ctx := context.Background()

	status := 200
	dur := 5
	if _, err := d.Create(ctx, &model.Log{
		Timestamp:  time.Now(),
		Level:      model.LevelInfo,
		Event:      model.EventHTTPRequest,
		Method:     "GET",
		Path:       "/api/v1/admin/logs",
		Status:     &status,
		DurationMs: &dur,
		ClientIP:   "127.0.0.1",
		RequestID:  "rid-1",
		Username:   "admin",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	items, total, err := d.List(ctx, model.LogQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected 1 row, got total=%d len=%d", total, len(items))
	}
	got := items[0]
	if got.Event != model.EventHTTPRequest || got.Method != "GET" || *got.Status != 200 || got.RequestID != "rid-1" || got.Username != "admin" {
		t.Fatalf("unexpected row: %+v", got)
	}
}

func TestLogDAO_ListFilters(t *testing.T) {
	d := newTestLogDAO(t)
	ctx := context.Background()
	now := time.Now()

	mk := func(level, event, method, path string, status int) *model.Log {
		s := status
		return &model.Log{Timestamp: now, Level: level, Event: event, Method: method, Path: path, Status: &s}
	}
	for _, l := range []*model.Log{
		mk(model.LevelInfo, model.EventHTTPRequest, "GET", "/api/v1/admin/logs", 200),
		mk(model.LevelWarn, model.EventHTTPRequest, "POST", "/api/v1/images", 404),
		mk(model.LevelError, model.EventPanic, "POST", "/api/v1/admin/images", 500),
		mk(model.LevelInfo, model.EventImageUpload, "POST", "/api/v1/admin/images", 200),
	} {
		if _, err := d.Create(ctx, l); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	tests := []struct {
		name      string
		q         model.LogQuery
		wantTotal int
	}{
		{"level error", model.LogQuery{Level: model.LevelError, Limit: 10}, 1},
		{"event upload", model.LogQuery{Event: model.EventImageUpload, Limit: 10}, 1},
		{"method POST", model.LogQuery{Method: "POST", Limit: 10}, 3},
		{"status 5xx", model.LogQuery{StatusClass: "5xx", Limit: 10}, 1},
		{"status 4xx", model.LogQuery{StatusClass: "4xx", Limit: 10}, 1},
		{"status 2xx", model.LogQuery{StatusClass: "2xx", Limit: 10}, 2},
		{"keyword images", model.LogQuery{Keyword: "images", Limit: 10}, 3},
		{"keyword logs", model.LogQuery{Keyword: "logs", Limit: 10}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, total, err := d.List(ctx, tt.q)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if total != tt.wantTotal {
				t.Fatalf("total=%d want %d", total, tt.wantTotal)
			}
		})
	}
}

func TestLogDAO_CountByRange(t *testing.T) {
	d := newTestLogDAO(t)
	ctx := context.Background()
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	yesterday := today.AddDate(0, 0, -1)

	for _, ts := range []time.Time{today, today, yesterday} {
		if _, err := d.Create(ctx, &model.Log{Timestamp: ts, Level: model.LevelInfo, Event: model.EventHTTPRequest}); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dayEnd := dayStart.AddDate(0, 0, 1)
	n, err := d.CountByRange(ctx, dayStart, dayEnd)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("today count=%d want 2", n)
	}
}

func TestLogDAO_BatchCreateAndClearAll(t *testing.T) {
	d := newTestLogDAO(t)
	ctx := context.Background()

	logs := []*model.Log{
		{Timestamp: time.Now(), Level: model.LevelInfo, Event: model.EventHTTPRequest, Method: "GET"},
		{Timestamp: time.Now(), Level: model.LevelInfo, Event: model.EventHTTPRequest, Method: "POST"},
	}
	if err := d.BatchCreate(ctx, logs); err != nil {
		t.Fatalf("batch create: %v", err)
	}
	_, total, err := d.List(ctx, model.LogQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 {
		t.Fatalf("total=%d want 2", total)
	}

	n, err := d.ClearAll(ctx)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if n != 2 {
		t.Fatalf("deleted=%d want 2", n)
	}
	_, total, _ = d.List(ctx, model.LogQuery{Limit: 10})
	if total != 0 {
		t.Fatalf("after clear total=%d want 0", total)
	}
}

// TestLogDAO_ClearInfoGet 验证条件删除只命中 info 级 GET 日志：
// warn/error 级、非 GET 方法、无 method 的业务事件均保留。
func TestLogDAO_ClearInfoGet(t *testing.T) {
	d := newTestLogDAO(t)
	ctx := context.Background()
	now := time.Now()

	mk := func(level, event, method string) *model.Log {
		return &model.Log{Timestamp: now, Level: level, Event: event, Method: method}
	}
	for _, l := range []*model.Log{
		mk(model.LevelInfo, model.EventHTTPRequest, "GET"),  // 命中
		mk(model.LevelInfo, model.EventHTTPRequest, "POST"), // info 但非 GET，保留
		mk(model.LevelWarn, model.EventHTTPRequest, "GET"),  // GET 但非 info，保留
		mk(model.LevelError, model.EventPanic, ""),          // 无 method，保留
		mk(model.LevelInfo, model.EventImageUpload, ""),     // 业务事件无 method，保留
	} {
		if _, err := d.Create(ctx, l); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	n, err := d.ClearInfoGet(ctx)
	if err != nil {
		t.Fatalf("clear info get: %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted=%d want 1", n)
	}

	_, total, err := d.List(ctx, model.LogQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 4 {
		t.Fatalf("after clear total=%d want 4", total)
	}
	// info+GET 组合应查不到了。
	_, getTotal, err := d.List(ctx, model.LogQuery{Level: model.LevelInfo, Method: "GET", Limit: 10})
	if err != nil {
		t.Fatalf("list info get: %v", err)
	}
	if getTotal != 0 {
		t.Fatalf("info GET residual=%d want 0", getTotal)
	}
}
