package model

// DashboardOverview 是仪表盘聚合统计接口的返回结构，一次性承载首页所需的全部指标。
//
// 由 DashboardService.Overview 聚合各 DAO 的查询结果组装而成，前端仪表盘页面据此渲染
// 统计卡片、近 N 天新增图片趋势图与快捷入口。RecentUploadTrend 用 DashboardTrendDay，
// 在 DailyCount 的 {date, count} 基础上追加按密钥拆分的 Keys，结构兼容日志直方图 buckets，
// 便于前端共用 LogsHistogram 组件（日志直方图不携带 Keys，tooltip 不渲染来源明细）。
type DashboardOverview struct {
	// ImagesTotal 是图片总量（无过滤）。
	ImagesTotal int64 `json:"images_total"`
	// StorageBytes 是全部图片 size 字段之和（字节），即「已登记图片总大小」。
	// 取自 DB SUM(size) 而非文件系统遍历，images 表为单一事实来源。
	StorageBytes int64 `json:"storage_bytes"`
	// APIKeysTotal 是密钥总数（含已吊销；已删除为物理删除，不在统计内）。
	APIKeysTotal int `json:"apikeys_total"`
	// APIKeysActive 是未吊销的有效密钥数。
	APIKeysActive int `json:"apikeys_active"`
	// APIKeysRevoked 是已吊销密钥数。
	APIKeysRevoked int `json:"apikeys_revoked"`
	// LogsTotal 是日志总量。
	LogsTotal int64 `json:"logs_total"`
	// RecentUploadTrend 是近 N 天每日新增图片数（按日期升序、缺日补零），
	// 每个单元额外携带按密钥拆分的 Keys，供仪表盘 tooltip 展示「当日每个 key 上传数」。
	// 与日志直方图 buckets 的 {date, count} 结构兼容，可直接喂给 LogsHistogram 组件。
	RecentUploadTrend []DashboardTrendDay `json:"recent_upload_trend"`
	// RecentUploadTotal 是近 N 天新增图片合计，供趋势图标题区高亮展示。
	RecentUploadTotal int `json:"recent_upload_total"`
	// Days 是趋势窗口天数（默认 30），回显给前端用于文案。
	Days int `json:"days"`
}

// KeyCount 是某日某来源（密钥 / 后台直传）的新增图片计数，名称已解析，供仪表盘 tooltip 渲染。
type KeyCount struct {
	// Name 是密钥标签；后台 JWT 直传的图片无关联密钥，统一展示为 "admin"。
	Name string `json:"name"`
	// Count 是该来源当日新增图片数。
	Count int `json:"count"`
}

// DashboardTrendDay 是仪表盘近 N 天趋势的单日单元，比 DailyCount 多携带按来源拆分。
//
// 复用 DailyCount 的 {Date, Count} 语义并追加可选 Keys：前端 LogsHistogram 组件对
// {date, count} 结构兼容，仅当 Keys 非空时在 tooltip 中渲染来源明细（日志直方图不携带 Keys）。
type DashboardTrendDay struct {
	// Date 是日期，YYYY-MM-DD。
	Date string `json:"date"`
	// Count 是当日新增图片合计（各来源之和）。
	Count int `json:"count"`
	// Keys 是当日按来源拆分的新增图片数（按 Count 降序）；当日无上传时为 nil（序列化时省略）。
	Keys []KeyCount `json:"keys,omitempty"`
}

// KeyGroupCount 是 DAO 按密钥分组的新增图片计数，KeyID 未解析名称，由 service 解析。
//
// DAO 只感知 KeyID（图片表外键），不感知密钥标签；service 据 apiKeyDAO.List 构建
// id->name 映射后解析为 KeyCount。KeyID 为 nil 表示后台 JWT 直传（展示为 "admin"）。
type KeyGroupCount struct {
	// KeyID 是添加该图片的密钥 ID，nil 表示后台 JWT 直传。
	KeyID *int
	// Count 是该密钥当日新增图片数。
	Count int
}
