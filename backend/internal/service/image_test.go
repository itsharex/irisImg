package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"testing"
	"time"

	"github.com/Lestine-Yan/irisImg/backend/config"
	"github.com/Lestine-Yan/irisImg/backend/internal/dao"
	"github.com/Lestine-Yan/irisImg/backend/internal/model"
	"github.com/Lestine-Yan/irisImg/backend/internal/pkg/storage"
)

// memImageDAO 是 dao.ImageDAO 的内存实现，供 service 单测使用。
// 故意只实现 Create / GetByHash / GetByID 这三个会被命中的方法。
type memImageDAO struct {
	byHash map[string]*model.Image
	byID   map[int]*model.Image
	nextID int
}

func newMemDAO() *memImageDAO {
	return &memImageDAO{
		byHash: map[string]*model.Image{},
		byID:   map[int]*model.Image{},
	}
}

func (m *memImageDAO) Create(_ context.Context, img *model.Image) (*model.Image, error) {
	m.nextID++
	img.ID = m.nextID
	cp := *img
	m.byHash[cp.Hash] = &cp
	m.byID[cp.ID] = &cp
	return &cp, nil
}

func (m *memImageDAO) GetByID(_ context.Context, id int) (*model.Image, error) {
	if v, ok := m.byID[id]; ok {
		return v, nil
	}
	return nil, dao.ErrNotFound
}

func (m *memImageDAO) GetByHash(_ context.Context, hash string) (*model.Image, error) {
	if v, ok := m.byHash[hash]; ok {
		return v, nil
	}
	return nil, dao.ErrNotFound
}

func (m *memImageDAO) List(_ context.Context, _ model.ImageListQuery) ([]*model.Image, int, error) {
	return nil, 0, errors.New("not used")
}

func (m *memImageDAO) Delete(_ context.Context, _ int) error {
	return errors.New("not used")
}

// ListByKeyID 返回指定密钥关联的全部图片（内存实现，供删除密钥级联测试使用）。
func (m *memImageDAO) ListByKeyID(_ context.Context, keyID int) ([]*model.Image, error) {
	var items []*model.Image
	for _, img := range m.byID {
		if img.KeyID != nil && *img.KeyID == keyID {
			items = append(items, img)
		}
	}
	return items, nil
}

// DeleteByKeyID 批量删除指定密钥关联的全部图片记录，返回删除条数。
func (m *memImageDAO) DeleteByKeyID(_ context.Context, keyID int) (int, error) {
	n := 0
	for id, img := range m.byID {
		if img.KeyID != nil && *img.KeyID == keyID {
			delete(m.byID, id)
			delete(m.byHash, img.Hash)
			n++
		}
	}
	return n, nil
}

// Count 返回内存中图片总量（仪表盘统计用）。
func (m *memImageDAO) Count(_ context.Context) (int64, error) {
	return int64(len(m.byID)), nil
}

// TotalSize 返回内存中全部图片 size 之和（仪表盘存储占用用）。
func (m *memImageDAO) TotalSize(_ context.Context) (int64, error) {
	var sum int64
	for _, img := range m.byID {
		sum += img.Size
	}
	return sum, nil
}

// CountByRange 统计 [start, end) 内按 created_at 新增的图片数（仪表盘趋势用）。
func (m *memImageDAO) CountByRange(_ context.Context, start, end time.Time) (int64, error) {
	var n int64
	for _, img := range m.byID {
		// 左闭右开：start <= CreatedAt < end
		if !img.CreatedAt.Before(start) && img.CreatedAt.Before(end) {
			n++
		}
	}
	return n, nil
}

// CountByRangeGrouped 统计 [start, end) 内按 key_id 分组的新增图片数（仪表盘按来源拆分用）。
// 与 CountByRange 同样的左闭右开区间判定；nil KeyID 归入 admin（KeyID 指针为 nil）。
func (m *memImageDAO) CountByRangeGrouped(_ context.Context, start, end time.Time) ([]model.KeyGroupCount, error) {
	type agg struct {
		keyID *int
		count int
	}
	idx := map[int]int{} // keyID 值（0=admin 哨兵）-> groups 下标
	var groups []agg
	for _, img := range m.byID {
		if img.CreatedAt.Before(start) || !img.CreatedAt.Before(end) {
			continue
		}
		id := 0
		if img.KeyID != nil {
			id = *img.KeyID
		}
		if pos, ok := idx[id]; ok {
			groups[pos].count++
		} else {
			idx[id] = len(groups)
			var kid *int
			if id != 0 {
				v := id
				kid = &v
			}
			groups = append(groups, agg{keyID: kid, count: 1})
		}
	}
	out := make([]model.KeyGroupCount, 0, len(groups))
	for _, g := range groups {
		out = append(out, model.KeyGroupCount{KeyID: g.keyID, Count: g.count})
	}
	return out, nil
}

// 编译期断言：memImageDAO 满足 dao.ImageDAO。
var _ dao.ImageDAO = (*memImageDAO)(nil)

func newTestImageService(t *testing.T) (*ImageService, *memImageDAO) {
	t.Helper()
	cfg := config.StorageConfig{
		RootDir:         filepath.Join(t.TempDir(), "imgs"),
		PublicBaseURL:   "",
		MaxUploadSizeMB: 1,
		AllowedMimeTypes: []string{
			"image/png", "image/jpeg", "image/gif", "image/webp",
		},
	}
	saver, err := storage.NewSaver(cfg)
	if err != nil {
		t.Fatalf("new saver: %v", err)
	}
	mem := newMemDAO()
	return NewImageService(mem, saver, cfg), mem
}

// makePNG 生成一张固定 2x2 大小的 PNG，用于 service 测试。
func makePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestImageService_Upload_Success(t *testing.T) {
	svc, mem := newTestImageService(t)
	content := makePNG(t)

	keyID := 42
	got, err := svc.Upload(context.Background(), &model.UploadImageInput{
		Filename: "cat.png",
		Content:  content,
		KeyID:    &keyID,
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	if got.ID == 0 {
		t.Fatalf("expected non-zero id")
	}
	if got.MimeType != "image/png" {
		t.Fatalf("mime = %q", got.MimeType)
	}
	if got.Width != 2 || got.Height != 2 {
		t.Fatalf("size = %dx%d", got.Width, got.Height)
	}
	if got.URL == "" || got.StoredPath == "" {
		t.Fatalf("expected URL & StoredPath filled: %+v", got)
	}
	if got.KeyID == nil || *got.KeyID != 42 {
		t.Fatalf("key id not propagated: %+v", got.KeyID)
	}
	if got.Hash == "" {
		t.Fatalf("hash should be set")
	}

	// 数据库里应当存在一条记录
	if _, ok := mem.byHash[got.Hash]; !ok {
		t.Fatalf("expected hash to be indexed in dao")
	}
}

func TestImageService_Upload_DedupSecondTime(t *testing.T) {
	svc, mem := newTestImageService(t)
	content := makePNG(t)

	keyID := 1
	first, err := svc.Upload(context.Background(), &model.UploadImageInput{
		Filename: "a.png", Content: content, KeyID: &keyID,
	})
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}

	// 同张图换个文件名再传一次，应该秒传命中、返回与首次相同的 ID。
	second, err := svc.Upload(context.Background(), &model.UploadImageInput{
		Filename: "different-name.png", Content: content, KeyID: &keyID,
	})
	if err != nil {
		t.Fatalf("second upload: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected dedup hit; first=%d second=%d", first.ID, second.ID)
	}
	if mem.nextID != 1 {
		t.Fatalf("expected only 1 row, got nextID=%d", mem.nextID)
	}
}

// TestImageService_Upload_AdminNilKey 覆盖后台 JWT 直传路径：
// KeyID 传 nil 时，落库记录的 KeyID 也应保持 nil（admin 直传，不关联密钥）。
func TestImageService_Upload_AdminNilKey(t *testing.T) {
	svc, mem := newTestImageService(t)
	content := makePNG(t)

	got, err := svc.Upload(context.Background(), &model.UploadImageInput{
		Filename: "admin.png",
		Content:  content,
		KeyID:    nil,
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if got.KeyID != nil {
		t.Fatalf("expected nil key_id for admin upload, got %v", *got.KeyID)
	}
	// 落库记录同样应为 nil。
	if stored, ok := mem.byHash[got.Hash]; !ok || stored.KeyID != nil {
		t.Fatalf("expected nil key_id in dao record, got %+v", stored.KeyID)
	}
}

func TestImageService_Upload_Errors(t *testing.T) {
	svc, _ := newTestImageService(t)
	ctx := context.Background()

	t.Run("empty file", func(t *testing.T) {
		_, err := svc.Upload(ctx, &model.UploadImageInput{Filename: "a.png", Content: nil})
		if !errors.Is(err, ErrEmptyFile) {
			t.Fatalf("expected ErrEmptyFile, got %v", err)
		}
	})

	t.Run("too large", func(t *testing.T) {
		big := make([]byte, svc.MaxBytes()+1)
		// 头部填一些 PNG signature 让嗅探不至于直接判 binary（其实不重要，
		// 因为大小校验在嗅探之前）。
		copy(big, []byte("\x89PNG\r\n\x1a\n"))
		_, err := svc.Upload(ctx, &model.UploadImageInput{Filename: "big.png", Content: big})
		if !errors.Is(err, ErrFileTooLarge) {
			t.Fatalf("expected ErrFileTooLarge, got %v", err)
		}
	})

	t.Run("unsupported mime", func(t *testing.T) {
		// 一段普通 ASCII 文本，DetectContentType 会返回 text/plain。
		_, err := svc.Upload(ctx, &model.UploadImageInput{
			Filename: "a.png",
			Content:  []byte("plain text content, not an image at all"),
		})
		if !errors.Is(err, ErrUnsupportedMime) {
			t.Fatalf("expected ErrUnsupportedMime, got %v", err)
		}
	})
}
