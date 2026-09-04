package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	// 注册 png/jpeg/gif 的 DecodeConfig；webp 等格式未注册时 image.DecodeConfig
	// 会返回错误，service 层兜底为 width/height = 0，不影响上传成功。
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strings"
	"time"

	"github.com/Lestine-Yan/irisImg/backend/config"
	"github.com/Lestine-Yan/irisImg/backend/internal/dao"
	"github.com/Lestine-Yan/irisImg/backend/internal/model"
	"github.com/Lestine-Yan/irisImg/backend/internal/pkg/storage"
)

// 图片上传相关的 sentinel 错误，供 api 层用 errors.Is 区分映射 HTTP 状态码。
var (
	// ErrEmptyFile 表示上传内容为空。
	ErrEmptyFile = errors.New("empty upload")
	// ErrFileTooLarge 表示上传内容超过 storage.max_upload_size_mb。
	// 注：api 层会优先用 http.MaxBytesReader 把超限拦在更早阶段，
	// 这里是 service 内部的二次防御。
	ErrFileTooLarge = errors.New("file too large")
	// ErrUnsupportedMime 表示嗅探出的真实 MIME 不在白名单内。
	ErrUnsupportedMime = errors.New("unsupported mime type")
)

// ImageService 负责图片上传的业务编排：嗅探 → 去重 → 落盘 → 落库。
type ImageService struct {
	dao   dao.ImageDAO
	saver *storage.Saver
	cfg   config.StorageConfig
	// maxBytes 是单次上传字节上限，由 cfg.MaxUploadSizeMB 推导。
	maxBytes int64
	// allowedMime 是白名单的快速查表（小写）。
	allowedMime map[string]struct{}
}

// NewImageService 通过依赖注入构造 ImageService。
//
// 入参里同时收 dao / saver / cfg：cfg 中的部分字段（白名单、大小上限）
// 在构造期就预处理成快速查表，避免每次上传都重新解析。
func NewImageService(d dao.ImageDAO, s *storage.Saver, cfg config.StorageConfig) *ImageService {
	max := int64(cfg.MaxUploadSizeMB)
	if max <= 0 {
		max = 20 // 与配置默认值保持一致
	}
	maxBytes := max * 1024 * 1024

	allow := make(map[string]struct{}, len(cfg.AllowedMimeTypes))
	for _, m := range cfg.AllowedMimeTypes {
		m = strings.ToLower(strings.TrimSpace(m))
		if m != "" {
			allow[m] = struct{}{}
		}
	}

	return &ImageService{
		dao:         d,
		saver:       s,
		cfg:         cfg,
		maxBytes:    maxBytes,
		allowedMime: allow,
	}
}

// MaxBytes 返回单次上传字节上限，供 api 层为 http.MaxBytesReader 设置阈值。
func (s *ImageService) MaxBytes() int64 {
	return s.maxBytes
}

// Upload 是上传图片的主流程：
//  1. 大小 & 空文件校验
//  2. 嗅探真实 MIME 并对照白名单
//  3. 算 sha256，命中已存在记录直接秒传返回
//  4. 解析宽高（解析失败则记 0，不影响上传）
//  5. 写盘 → 生成对外 URL → 落库
func (s *ImageService) Upload(ctx context.Context, in *model.UploadImageInput) (*model.Image, error) {
	if in == nil || len(in.Content) == 0 {
		return nil, ErrEmptyFile
	}
	if int64(len(in.Content)) > s.maxBytes {
		return nil, ErrFileTooLarge
	}

	// 真实 MIME 嗅探。http.DetectContentType 看头部 512 字节，
	// 客户端伪造 Content-Type 没用。
	mime := http.DetectContentType(in.Content)
	// DetectContentType 可能返回带参数形式（如 "text/plain; charset=utf-8"），
	// 白名单只比对裸 MIME 字符串。
	mimeKey := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	if _, ok := s.allowedMime[mimeKey]; !ok {
		return nil, ErrUnsupportedMime
	}

	// 算 sha256（作为文件名 + 去重键）。
	sum := sha256.Sum256(in.Content)
	hash := hex.EncodeToString(sum[:])

	// 秒传：同 hash 已存在，直接复用记录，不重复写盘，不重复落库。
	if existing, err := s.dao.GetByHash(ctx, hash); err == nil {
		return existing, nil
	} else if !errors.Is(err, dao.ErrNotFound) {
		return nil, err
	}

	// 解析宽高（失败不阻断主流程）。
	width, height := decodeImageSize(in.Content)

	// 由 MIME 反推扩展名，不信任原始文件名后缀。
	ext := extFromMime(mimeKey)

	now := time.Now()
	relPath, err := s.saver.Save(in.Content, hash, ext, now)
	if err != nil {
		return nil, err
	}
	url := s.saver.PublicURL(relPath)

	img := &model.Image{
		Filename:   in.Filename,
		StoredPath: relPath,
		URL:        url,
		Size:       int64(len(in.Content)),
		MimeType:   mimeKey,
		Width:      width,
		Height:     height,
		Hash:       hash,
		KeyID:      in.KeyID,
	}
	return s.dao.Create(ctx, img)
}

// List 查询图片列表，按 ImageListQuery 过滤 / 排序 / 分页。
// Limit<=0 时兜底为 24（与内容中心默认页大小一致），避免无限制拉取。
// 排序方向、key_id 过滤由 dao 层落实，service 只做参数兜底与结果组装。
func (s *ImageService) List(ctx context.Context, q model.ImageListQuery) (*model.ImageListResult, error) {
	if q.Limit <= 0 {
		q.Limit = 24
	}
	items, total, err := s.dao.List(ctx, q)
	if err != nil {
		return nil, err
	}
	return &model.ImageListResult{Items: items, Total: total}, nil
}

// BatchDelete 批量删除图片：物理文件 best-effort 删除 + 元信息记录批量删除。
// 返回 (实际删除条数, 实际删除的图片 ID 列表)；不存在的 ID 静默跳过（幂等语义）。
//
// 顺序：ListByIDs 取现存记录 → 逐张 best-effort 删物理文件（失败不阻断，Saver.Delete 幂等）
// → DeleteByIDs 一条 SQL 批量删记录。非事务（低频管理操作，与 APIKeyService.Delete 同款权衡）：
// 若删 DB 失败，个别物理文件已删但记录仍在，该图缩略图 404，管理员重删时幂等兜底。
func (s *ImageService) BatchDelete(ctx context.Context, ids []int) (int, []int, error) {
	if len(ids) == 0 {
		return 0, nil, nil
	}

	// 只删现存的记录：混入的不存在 ID 直接跳过，不构成错误。
	items, err := s.dao.ListByIDs(ctx, ids)
	if err != nil {
		return 0, nil, err
	}
	if len(items) == 0 {
		return 0, []int{}, nil
	}

	// 先逐张删物理文件（best-effort）：失败不阻断，避免个别文件权限等问题
	// 卡住整批删除；Saver.Delete 对已不存在的文件返回 nil，天然幂等。
	for _, img := range items {
		_ = s.saver.Delete(img.StoredPath)
	}

	deleted, err := s.dao.DeleteByIDs(ctx, ids)
	if err != nil {
		return 0, nil, err
	}

	deletedIDs := make([]int, 0, len(items))
	for _, img := range items {
		deletedIDs = append(deletedIDs, img.ID)
	}
	return deleted, deletedIDs, nil
}

// decodeImageSize 通过标准库 image.DecodeConfig 读宽高，未注册的格式或解析失败均返回 0,0。
func decodeImageSize(content []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// extFromMime 将常见图片 MIME 映射成不带点的扩展名。
// 命中白名单之外的 MIME 不会走到这里（已被上游拒绝）。
func extFromMime(mime string) string {
	switch mime {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	case "image/bmp":
		return "bmp"
	case "image/svg+xml":
		return "svg"
	default:
		// 兜底：取 MIME 的子类型部分，丢弃未知格式时仍能得到一个像样的扩展名。
		if i := strings.Index(mime, "/"); i >= 0 {
			return strings.ToLower(mime[i+1:])
		}
		return "bin"
	}
}
