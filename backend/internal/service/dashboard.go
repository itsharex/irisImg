package service

import (
	"context"
	"sort"
	"time"

	"github.com/Lestine-Yan/irisImg/backend/internal/dao"
	"github.com/Lestine-Yan/irisImg/backend/internal/model"
)

const dashboardTrendDays = 30 // 仪表盘近 N 天趋势默认天数

// DashboardService 聚合仪表盘首页所需的各项统计指标。
//
// 依赖 imageDAO / apiKeyDAO / logDAO 三个 DAO（沿用本仓库 service 仅依赖 DAO 的既有模式，
// 不依赖其他 service），在 Overview 中聚合图片总量、存储大小、密钥计数、日志总量
// 与近 N 天上传趋势，一次性返回给仪表盘页面。
type DashboardService struct {
	imageDAO  dao.ImageDAO
	apiKeyDAO dao.APIKeyDAO
	logDAO    dao.LogDAO
}

// NewDashboardService 构造 DashboardService。
func NewDashboardService(imageDAO dao.ImageDAO, apiKeyDAO dao.APIKeyDAO, logDAO dao.LogDAO) *DashboardService {
	return &DashboardService{imageDAO: imageDAO, apiKeyDAO: apiKeyDAO, logDAO: logDAO}
}

// Overview 聚合返回仪表盘首页所需的全部指标。
//
// days 控制近 N 天上传趋势窗口（<=0 兜底为默认 30）。趋势按日循环 imageDAO.CountByRangeGrouped，
// 照搬 LogService.Histogram 的按日聚合模式（本地时区午夜对齐、缺日补零、升序），并据 apiKeyDAO.List
// 解析每张图片的来源密钥标签（后台直传为 admin），供前端 tooltip 展示按来源拆分。
func (s *DashboardService) Overview(ctx context.Context, days int) (*model.DashboardOverview, error) {
	if days <= 0 {
		days = dashboardTrendDays
	}

	overview := &model.DashboardOverview{Days: days}

	// 图片总量与存储大小（DB SUM(size)）。
	imagesTotal, err := s.imageDAO.Count(ctx)
	if err != nil {
		return nil, err
	}
	overview.ImagesTotal = imagesTotal

	storageBytes, err := s.imageDAO.TotalSize(ctx)
	if err != nil {
		return nil, err
	}
	overview.StorageBytes = storageBytes

	// APIkey 计数：一次拉全量后内存按 Revoked 分桶，避免多次 Count 往返。
	// 同批 keys 顺带构建 id->name 映射，供 uploadTrend 解析每张图片的来源密钥标签。
	keys, err := s.apiKeyDAO.List(ctx)
	if err != nil {
		return nil, err
	}
	overview.APIKeysTotal = len(keys)
	nameByID := make(map[int]string, len(keys))
	for _, k := range keys {
		if k.Revoked {
			overview.APIKeysRevoked++
		}
		nameByID[k.ID] = k.Name
	}
	overview.APIKeysActive = overview.APIKeysTotal - overview.APIKeysRevoked

	// 日志总量。
	logsTotal, err := s.logDAO.Count(ctx)
	if err != nil {
		return nil, err
	}
	overview.LogsTotal = logsTotal

	// 近 N 天每日新增图片趋势（含按来源拆分，供 tooltip）。
	trend, trendTotal, err := s.uploadTrend(ctx, days, nameByID)
	if err != nil {
		return nil, err
	}
	overview.RecentUploadTrend = trend
	overview.RecentUploadTotal = trendTotal

	return overview, nil
}

// uploadTrend 返回最近 days 天的每日新增图片（按日期升序、缺日补零）与合计，每个单元携带按来源拆分。
//
// 与 LogService.Histogram 同构的按日循环：用 time.Now().Location() 构造今日午夜，逐日左闭右开区间
// 调 imageDAO.CountByRangeGrouped，缺日因查询返回空分组自然补零。nameByID 把分组 KeyID 解析为密钥标签
// （nil -> "admin"）；未命中兜底为「已删除密钥」（理论不会发生：删除密钥会级联删除其图片）。
// Keys 按 Count 降序，便于 tooltip 从高到低展示；当日无上传时 Keys 为 nil（序列化省略）。
func (s *DashboardService) uploadTrend(ctx context.Context, days int, nameByID map[int]string) ([]model.DashboardTrendDay, int, error) {
	now := time.Now()
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	out := make([]model.DashboardTrendDay, 0, days)
	total := 0
	for i := days - 1; i >= 0; i-- {
		dayStart := today.AddDate(0, 0, -i)
		dayEnd := dayStart.AddDate(0, 0, 1)
		groups, err := s.imageDAO.CountByRangeGrouped(ctx, dayStart, dayEnd)
		if err != nil {
			return nil, 0, err
		}
		dayTotal := 0
		var keys []model.KeyCount
		for _, g := range groups {
			dayTotal += g.Count
			name := "admin"
			if g.KeyID != nil {
				if n, ok := nameByID[*g.KeyID]; ok {
					name = n
				} else {
					name = "已删除密钥"
				}
			}
			keys = append(keys, model.KeyCount{Name: name, Count: g.Count})
		}
		sort.Slice(keys, func(a, b int) bool { return keys[a].Count > keys[b].Count })
		out = append(out, model.DashboardTrendDay{
			Date:  dayStart.Format("2006-01-02"),
			Count: dayTotal,
			Keys:  keys, // 无上传时为 nil -> omitempty 省略
		})
		total += dayTotal
	}
	return out, total, nil
}
