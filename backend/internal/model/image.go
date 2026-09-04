package model

import "time"

// Image 是图片元信息的跨层数据载体（实体）。
//
// 它独立于 Ent 生成的 ent.Image：DAO 层负责在二者之间转换，
// 这样 service / api 层不直接依赖 Ent，便于后续替换存储实现。
type Image struct {
	ID         int       `json:"id"`
	Filename   string    `json:"filename"`    // 上传时的原始文件名
	StoredPath string    `json:"stored_path"` // 相对存储根目录的落盘路径
	URL        string    `json:"url"`         // 对外访问地址
	Size       int64     `json:"size"`        // 文件大小（字节）
	MimeType   string    `json:"mime_type"`   // MIME 类型，如 image/png
	Width      int       `json:"width"`       // 宽度（像素），未知为 0
	Height     int       `json:"height"`      // 高度（像素），未知为 0
	Hash       string    `json:"hash"`        // 内容哈希，用于去重
	CreatedAt  time.Time `json:"created_at"`  // 创建时间
	// KeyID 是添加该图片的 API 密钥 ID，可空：
	// 通过后台 JWT 上传的图片没有关联密钥，此处为 nil。
	KeyID *int `json:"key_id,omitempty"`
}

// UploadImageInput 描述「上传一张图片」的入参，由 api 层从 HTTP 请求装配后传给 service。
//
// 设计成结构体而不是多参数，便于后续追加字段（如标签、相册 ID）不破坏签名。
type UploadImageInput struct {
	// Filename 是上传时客户端给出的原始文件名，仅作展示。
	// 真实落盘文件名由 hash + 嗅探得到的扩展名决定，不信任此字段。
	Filename string
	// Content 是图片完整字节，由 api 层在受 MaxBytesReader 保护下读出。
	Content []byte
	// KeyID 是添加该图片的 API 密钥 ID。
	// 通过 API Key 渠道上传时由中间件保证非空；后台 JWT 直传时可置 nil。
	KeyID *int
}

// ImageListQuery 描述「查询图片列表」的过滤 / 排序 / 分页条件。
//
// 设计成结构体而不是多参数，便于后续追加过滤维度（如 MIME、时间区间）不破坏签名。
type ImageListQuery struct {
	// KeyID 为非 nil 时仅返回该密钥添加的图片；为 nil 表示不按密钥过滤（全部）。
	KeyID *int
	// Order 控制按 created_at 排序的方向：非 "desc" 一律视为升序（"asc"）。
	// 内容中心默认需要时间升序，故空字符串也按升序处理。
	Order string
	// Offset / Limit 是分页参数；Limit<=0 时由 service 兜底为默认页大小。
	Offset int
	Limit  int
}

// ImageListResult 是图片列表查询的返回结构，同时承载分页元信息。
type ImageListResult struct {
	Items []*Image `json:"items"`
	Total int      `json:"total"`
}

// BatchDeleteImagesRequest 是内容中心批量删除图片的请求体。
// 复用吊销/删除密钥同款账号密码二次确认机制：api 层用 AuthService.VerifyCredentials
// 常量时间比对校验，失败返回 403（而非 401），避免触发前端全局登出。
// IDs 上限 100，防止误操作一次删除巨量图片。
type BatchDeleteImagesRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	// IDs 是待删除的图片主键列表：非空、每项 > 0、上限 100。
	IDs []int `json:"ids" binding:"required,min=1,max=100,dive,gt=0"`
}

// BatchDeleteImagesResponse 是批量删除图片的响应体。
// 不存在的 ID 静默跳过（幂等语义），两个均按「实际删除」口径返回，
// 供前端同步本地列表状态与分页计算。
type BatchDeleteImagesResponse struct {
	// Deleted 是实际删除的记录条数（不存在的 ID 不计入）。
	Deleted int `json:"deleted"`
	// IDs 是实际被删除的图片 ID 列表。
	IDs []int `json:"ids"`
}
