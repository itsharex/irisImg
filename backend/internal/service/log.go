package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Lestine-Yan/irisImg/backend/internal/dao"
	"github.com/Lestine-Yan/irisImg/backend/internal/model"
	"github.com/Lestine-Yan/irisImg/backend/internal/pkg/logger"
	"go.uber.org/zap"
)

// LogRecorder 是日志记录的窄接口，供需要发射业务事件的控制器依赖（避免依赖完整 LogService）。
type LogRecorder interface {
	Record(l *model.Log)
}

const (
	logBufferSize    = 2048  // 异步缓冲通道容量
	logBatchSize     = 200   // 单批最大写入条数
	logFlushInterval = time.Second
	logQueryDefault  = 50    // 列表默认页大小
	logHistogramDays = 14    // 直方图默认天数
)

// LogService 负责日志的异步落库与查询 / 直方图 / 清理。
//
// 写入走异步批量：Record 非阻塞地把日志推入缓冲通道，后台 flusher 协程按批或按秒
// 调 dao.BatchCreate 落库，使请求处理零 DB 写延迟；缓冲满则丢弃并告警，绝不阻塞请求。
// 查询 / 直方图 / 清理走同步 dao 调用。
//
// 关闭安全：不关闭 buf 通道，改用 done 通道通知 flusher 退出。Record 在 done 关闭后
// 经 select 的 done 分支直接返回，绝不触发 send on closed channel panic；故 main 在
// srv.Shutdown 超时后仍可安全调用 Close（即便有在途 handler 仍在 Record）。
type LogService struct {
	dao      dao.LogDAO
	logger   *logger.Logger
	buf      chan *model.Log
	done     chan struct{}
	flushReq chan chan struct{}
	wg       sync.WaitGroup
}

// NewLogService 构造 LogService 并启动后台 flusher 协程。
func NewLogService(d dao.LogDAO, l *logger.Logger) *LogService {
	s := &LogService{
		dao:      d,
		logger:   l,
		buf:      make(chan *model.Log, logBufferSize),
		done:     make(chan struct{}),
		flushReq: make(chan chan struct{}),
	}
	s.wg.Add(1)
	go s.flushLoop()
	return s
}

// Record 非阻塞地把一条日志加入缓冲。
// 关闭后（done 已关闭）安全返回不 panic；缓冲满则丢弃并告警。
func (s *LogService) Record(l *model.Log) {
	if l == nil {
		return
	}
	if l.Timestamp.IsZero() {
		l.Timestamp = time.Now()
	}
	if l.Level == "" {
		l.Level = model.LevelInfo
	}
	select {
	case s.buf <- l:
	case <-s.done:
		// 已关闭：丢弃，不阻塞也不 panic。
	default:
		if s.logger != nil {
			s.logger.Warn(context.Background(), "log buffer full, record dropped", zap.String("event", l.Event))
		}
	}
}

// flushLoop 后台排空缓冲，按批或按秒批量落库。
// - 收到 flushReq：立即 flush 当前批次并回信（供 ClearAll 同步 flush）。
// - 收到 done（关闭）：排空剩余缓冲后 flush 再退出。
func (s *LogService) flushLoop() {
	defer s.wg.Done()
	batch := make([]*model.Log, 0, logBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.dao.BatchCreate(ctx, batch); err != nil && s.logger != nil {
			s.logger.Error(context.Background(), "batch insert logs failed", zap.Error(err), zap.Int("n", len(batch)))
		}
		cancel()
		batch = batch[:0]
	}
	ticker := time.NewTicker(logFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case l := <-s.buf:
			batch = append(batch, l)
			if len(batch) >= logBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case done := <-s.flushReq:
			// 先排空通道中已缓冲的日志再 flush，确保 flushSync 真正落盘所有在途日志。
		Drain:
			for {
				select {
				case l := <-s.buf:
					batch = append(batch, l)
				default:
					break Drain
				}
			}
			flush()
			close(done)
		case <-s.done:
			// 关闭：排空剩余缓冲后 flush，再退出。
			for {
				select {
				case l := <-s.buf:
					batch = append(batch, l)
				default:
					flush()
					return
				}
			}
		}
	}
}

// flushSync 同步排空当前缓冲并落库，供 ClearAll 在删除前确保在途日志已落盘。
// 若 flusher 已退出（done 关闭）则直接返回。
func (s *LogService) flushSync() {
	done := make(chan struct{})
	select {
	case s.flushReq <- done:
		<-done
	case <-s.done:
	}
}

// Close 通知 flusher 关闭：排空并 flush 剩余日志后退出。可安全在 Record 仍在被调用时关闭。
func (s *LogService) Close() {
	close(s.done)
	s.wg.Wait()
}

// List 分页查询日志（按 timestamp 倒序）。
func (s *LogService) List(ctx context.Context, q model.LogQuery) ([]*model.Log, int, error) {
	if q.Limit <= 0 {
		q.Limit = logQueryDefault
	}
	return s.dao.List(ctx, q)
}

// Histogram 返回最近 days 天的每日计数（按日期升序、缺日补零），以及总条数。
func (s *LogService) Histogram(ctx context.Context, days int) ([]model.DailyCount, int, error) {
	if days <= 0 {
		days = logHistogramDays
	}
	now := time.Now()
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	out := make([]model.DailyCount, 0, days)
	total := 0
	for i := days - 1; i >= 0; i-- {
		dayStart := today.AddDate(0, 0, -i)
		dayEnd := dayStart.AddDate(0, 0, 1)
		c, err := s.dao.CountByRange(ctx, dayStart, dayEnd)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, model.DailyCount{Date: dayStart.Format("2006-01-02"), Count: c})
		total += c
	}
	return out, total, nil
}

// ClearAll 先同步 flush 在途日志、再清空全表，最后补记 log.clear 审计事件。
// 先 flush 再删除可避免缓冲中的旧日志在清空后被 flusher 重新写回库。
func (s *LogService) ClearAll(ctx context.Context, lc model.LogContext) (int64, error) {
	s.flushSync()
	n, err := s.dao.ClearAll(ctx)
	if err != nil {
		return 0, err
	}
	s.Record(model.NewEventLog(model.EventLogClear, model.LevelInfo, fmt.Sprintf("cleared %d logs", n), lc))
	return n, nil
}

// ClearInfoGet 先同步 flush 在途日志、再删除全部 info 级 GET 请求日志，最后补记 log.clear_get 审计事件。
// flush 时机与 ClearAll 相同；审计事件本身 method 为 NULL，不会被自己的删除条件命中。
func (s *LogService) ClearInfoGet(ctx context.Context, lc model.LogContext) (int64, error) {
	s.flushSync()
	n, err := s.dao.ClearInfoGet(ctx)
	if err != nil {
		return 0, err
	}
	s.Record(model.NewEventLog(model.EventLogClearGet, model.LevelInfo, fmt.Sprintf("cleared %d info GET logs", n), lc))
	return n, nil
}
