# 图片上传 / 静态反代说明（irisImg 后端）

> 本文档跨文件讲清楚「一张图片是怎么从客户端跑到磁盘、再被外部 URL 访问到的」。
> 各 `.go` 文件的逐文件文档见各自目录的 `.md`；API 密钥鉴权链路见 [`APIKEY.md`](./APIKEY.md)。

---

## 1. 整体设计

- 上传通道有两条，独立解耦：
  - **对外**：`POST /images`，挂在 **API 密钥鉴权**之下，由 [`middleware.APIKeyAuth`](./internal/middleware/apikey.md) 保证 POST 必为 `readwrite` 密钥，落库 `key_id` 记录是哪把密钥添加的。
  - **后台**：`POST /admin/images`，挂在 **JWT 鉴权**之下，供内容中心管理端直传，`key_id` 留空（admin 直传）。两条通道复用同一套业务流程（嗅探 → 去重 → 落盘 → 落库）。
- 文件名由内容 **SHA256** 计算得出，**天然唯一、天然去重**：同一张图二次上传自动秒传，复用首次记录。
- 落盘目录按 `<root>/<YYYY>/<MM>/<sha256>.<ext>` 排布，避免单目录文件过多。
- **真实 MIME 嗅探**（`http.DetectContentType`）+ 白名单，不信任客户端 `Content-Type`，扩展名由后端推导。
- **对外访问 URL** 由配置 `storage.public_base_url` + 相对路径拼接：空 → `/imgs/...`（前端/Nginx 同域反代）；填了如 `https://img.example.com` → 绝对地址。
- 元数据落库 `images` 表（含 `key_id`），记录是哪把密钥添加的图片，便于审计与后续按密钥维度展示。
- **删除**：`DELETE /admin/images`（JWT + HTTPSOnly + 账号密码二次确认）批量删除，物理文件与记录同删；不存在的 ID 静默跳过（幂等），删除时记录 `image.delete`（warn）业务事件。

参与的代码文件：

| 角色 | 文件 |
| --- | --- |
| Schema | `ent/schema/image.go` |
| 配置 | `config/config.go`、`config/config.yaml`（`storage` 段） |
| 存储工具 | `internal/pkg/storage/storage.go` |
| DTO | `internal/model/image.go`（`Image` / `UploadImageInput` / `BatchDeleteImagesRequest` / `BatchDeleteImagesResponse`） |
| DAO | `internal/dao/dao.go`、`internal/dao/entdao/image.go` |
| 业务逻辑 | `internal/service/image.go` |
| 中间件 | `internal/middleware/apikey.go`（鉴权 + 限流，已存在） |
| 控制器 | `internal/api/image.go` |
| 统一响应 | `internal/pkg/response/response.go`（`CodePayloadTooLarge` / `PayloadTooLarge`） |
| 路由装配 | `internal/router/router.go` |
| 静态服务 | `internal/router/static.go`（`/imgs` 扩展名白名单 handler，纵深防御） |
| 启动入口 | `cmd/server/main.go`（启动期构造 `storage.Saver`） |

## 2. 配置

```yaml
storage:
  root_dir: "data/imgs"             # 落盘根目录；相对路径相对进程 cwd，生产建议改绝对路径
  public_base_url: ""               # 空 → 返回 /imgs/<rel>，前端/Nginx 同域反代
                                    # 非空 → 例如 "https://img.example.com"（须带协议，裸域名会自动补 https://；结尾不带 /）
  max_upload_size_mb: 20            # 单次上传字节上限（MiB），<=0 回退 20
  allowed_mime_types:               # 真实 MIME 白名单（后端嗅探，不信任客户端 Content-Type）
    - "image/png"
    - "image/jpeg"
    - "image/gif"
    - "image/webp"
```

详见 [`config.md`](./config/config.md)。

## 3. 接口

### `POST /api/v1/images` —— 添加图片

- **鉴权**：`X-API-Key: <readwrite 密钥>`（由 [`middleware.APIKeyAuth`](./internal/middleware/apikey.md) 校验；只读密钥被该中间件直接 403）。
- **请求体**：`multipart/form-data`，唯一字段 `file`（图片二进制）。
- **成功响应**：`200` + `data` 为完整 `model.Image`（`id / url / stored_path / size / mime_type / width / height / hash / key_id / created_at`）。
- **错误**：

| HTTP | 业务码 | 场景 |
| --- | --- | --- |
| 400 | `CodeBadRequest` | 缺少 `file` 字段 / 内容为空 / 嗅探出的 MIME 不在白名单 |
| 401 | `CodeAPIKeyMissing` / `CodeAPIKeyInvalid` | 缺密钥 / 格式非法 / 已吊销 |
| 403 | `CodeForbidden` | 只读密钥写入 |
| 413 | `CodePayloadTooLarge` | 超过 `storage.max_upload_size_mb` |
| 429 | `CodeTooManyRequests` | 触发该密钥限流 |
| 500 | `CodeServerError` | 落盘 / 落库失败等内部错误 |

### `GET /api/v1/admin/images` —— 后台图片列表

- **鉴权**：`Authorization: Bearer <JWT>`（由 [`middleware.JWTAuth`](./internal/middleware/auth.md) 校验），供后台内容中心调用，与对外 API Key 通道解耦。
- **Query**：`key_id`（可选，缺省=全部）、`order`（asc/desc，默认 asc 升序）、`page`（默认 1）、`page_size`（默认 24）。
- **响应**：`200` + `data` 为 `{ items: [model.Image], total, page, page_size }`。
- **错误**：400（page/page_size/key_id 非法）、401（未登录）、500（内部错误）。

### `POST /api/v1/admin/images` —— 后台直传图片

- **鉴权**：`Authorization: Bearer <JWT>`（由 [`middleware.JWTAuth`](./internal/middleware/auth.md) 校验），无需 `X-API-Key`。供内容中心在管理端直接上传，与对外 API Key 上传通道解耦。
- **请求体**：`multipart/form-data`，唯一字段 `file`（图片二进制）。
- **关联密钥**：**不关联**——`key_id` 留空（NULL），语义上即「admin 直传」。这类图片只会在内容中心「全部」里出现，按密钥筛选时不可见；详情里来源展示为 `admin`。
- **成功响应**：`200` + `data` 为完整 `model.Image`（`key_id` 为 `null`，因 `omitempty` 不出现在 JSON 中）。
- **错误**：

| HTTP | 业务码 | 场景 |
| --- | --- | --- |
| 400 | `CodeBadRequest` | 缺少 `file` 字段 / 内容为空 / 嗅探出的 MIME 不在白名单 |
| 401 | `CodeUnauthorized` | 未登录 / JWT 失效（由中间件返回） |
| 413 | `CodePayloadTooLarge` | 超过 `storage.max_upload_size_mb` |
| 500 | `CodeServerError` | 落盘 / 落库失败等内部错误 |

> 业务流程（嗅探 → sha256 秒传 → 落盘 → 落库）与 `POST /images` 完全一致，复用 `service.ImageService.Upload`，差别仅在 `KeyID` 传 `nil`。详见 [`internal/api/image.md`](./internal/api/image.md) 的 `CreateAdmin`。

### `DELETE /api/v1/admin/images` —— 批量删除图片

- **鉴权**：`Authorization: Bearer <JWT>` + [`HTTPSOnly`](./internal/middleware/https.md)（生产环境由 `apikey.https_only` 开启）。供内容中心多选删除，物理文件与记录同删，不可恢复。
- **请求体**：JSON，`{ username, password, ids: []int }`。账号密码为二次确认（与吊销 / 删除密钥、清理日志同款机制），失败返回 **403** 而非 401，不触发前端全局登出；`ids` 非空、每项 > 0、上限 100。
- **成功响应**：`200` + `data` 为 `{ deleted: 实际删除条数, ids: 实际被删除的图片 ID 列表 }`。**不存在的 ID 静默跳过**（幂等语义），全部不存在时返回 `deleted: 0`。
- **删除顺序**：`ListByIDs` 取现存记录 → 逐张 best-effort 删物理文件（失败不阻断，`Saver.Delete` 幂等）→ `DeleteByIDs` 一条 SQL 批量删记录。非事务（低频管理操作）；若删库失败个别文件已删，记录仍在，重删时幂等兜底。
- **审计**：删除成功记录 `image.delete`（warn）业务事件，消息为 `batch delete images: N`。
- **错误**：

| HTTP | 业务码 | 场景 |
| --- | --- | --- |
| 400 | `CodeBadRequest` | body 解析失败 / `ids` 为空 / 超过 100 / 含非正数 |
| 401 | `CodeUnauthorized` | 未登录 / JWT 失效（由中间件返回） |
| 403 | `CodeForbidden` | 账号密码二次确认失败（或生产环境 HTTPSOnly 拦截非 HTTPS 请求） |
| 500 | `CodeServerError` | 查询 / 删除失败等内部错误 |

详见 [`internal/api/image.md`](./internal/api/image.md) 的 `BatchDeleteAdmin` 与 [`internal/service/image.md`](./internal/service/image.md) 的 `BatchDelete`。

### `GET /api/v1/images` —— 申请图片（占位）

任意有效密钥可访问，**当前固定返回 501**，对外列表 / 单图查询语义待定。

## 4. 上传链路

```
client           api.ImageAPI             service.ImageService           dao.ImageDAO         pkg/storage.Saver
  │ POST /images    │                          │                              │                       │
  │ X-API-Key       │                          │                              │                       │
  │ form file=…     │                          │                              │                       │
  │ ───────────────►│ MaxBytesReader+FormFile  │                              │                       │
  │                 │ ──── Upload(input) ────► │                              │                       │
  │                 │                          │ DetectContentType + 白名单    │                       │
  │                 │                          │ sha256                       │                       │
  │                 │                          │ GetByHash ─────────────────► │                       │
  │                 │                          │ ◄───── (existing / NotFound) │                       │
  │                 │                          │ DecodeConfig (W/H, 失败=0,0)  │                       │
  │                 │                          │ Save(content, hash, ext) ─── │ ────────────────────► │
  │                 │                          │                              │       <root>/YYYY/MM  │
  │                 │                          │ PublicURL(rel)               │                       │
  │                 │                          │ Create(*model.Image) ──────► │                       │
  │                 │ ◄────── *model.Image ─── │                              │                       │
  │ ◄── 200 Body ───│                          │                              │                       │
```

### 关键节点

1. **MaxBytesReader 早拦**：`api.ImageAPI.Create` 用 `http.MaxBytesReader(c.Writer, c.Request.Body, svc.MaxBytes())` 把超大请求体拦在 Multipart 解析之前，节省内存。
2. **真实 MIME 嗅探**：service 内 `http.DetectContentType` 看头部 512 字节，剥掉 `;charset=...` 等参数后比对白名单。
3. **秒传**：`dao.GetByHash` 命中即直接返回该记录，**不重复写盘、不重复落库**（连 `key_id` 也保持首次写入的值；后续若需记录"谁又传过一次"再加关联表）。
4. **写盘原子**：`pkg/storage.Saver.Save` 写到同目录临时文件再 `os.Rename`，避免半写状态被读取。
5. **URL 拼接**：`Saver.PublicURL`；空 base_url 返回 `/imgs/<rel>`，配 base_url 返回 `<base>/<rel>`。
6. **落库**：`dao.ImageDAO.Create` 写入 `images` 表，并通过 schema 上的 `key` edge 关联到 `api_keys.id`。

## 5. 部署与 Nginx 反代约定

- **开发期**：后端 `router` 已注册 `/imgs/*filepath`（GET/HEAD），由 [`serveImages`](./internal/router/static.md) 服务 `storage.root_dir`，前端直接通过后端 origin（如 `http://localhost:8080/imgs/...`）即可加载图片，无需 Nginx。**带图片扩展名白名单**（由 `storage.allowed_mime_types` 折算，仅放行 `.png/.jpg/.jpeg/.gif/.webp` 等）：即便 `root_dir` 被误配成工作目录，未认证访客也无法经 `/imgs` 下载 `config.yaml` / `irisImg.db` / 源码；目录列表与 `..` 逃逸同样被挡回 404。启动期另有 [`storage.NewSaver`](./internal/pkg/storage.md) 的 `guardRootDir` 拒绝 `root_dir` 指向 cwd 本身/祖先的误配。前端 `useImages.resolveImageUrl` 会把相对 URL 拼成完整地址。
- `storage.root_dir` 与 Nginx `location /imgs/` 暴露的物理路径**必须一致**。例如：

  ```yaml
  storage:
    root_dir: "/var/lib/irisImg/imgs"
    public_base_url: ""
  ```

  ```nginx
  location /imgs/ {
    # 仅放行图片扩展名（与后端 serveImages 白名单对齐的纵深防御）：
    # 即便 root_dir 误配或目录混入非图片文件，也无法经 /imgs/ 下载 .yaml/.db/.go 等。
    if ($uri !~* \.(png|jpg|jpeg|gif|webp)$) {
      return 404;
    }
    alias /var/lib/irisImg/imgs/;
    expires 30d;
    add_header Cache-Control "public, immutable";
  }
  ```

- 想用独立图片域名（如 `https://img.example.com`）：把 `public_base_url` 配上（**结尾不带斜杠**），并在那个域名同样反代到 `root_dir`。
- 备份 / 迁移时同步处理 `root_dir` 与数据库；hash 文件名让"按需补图"也很简单。
- **Nginx 配置模板已随仓库 `deploy/nginx/` 提供**：`irisImg.conf.example`（HTTPS 生产形态，SNI 分流）与 `irisImg.http.conf.example`（HTTP 最简版），首次部署 `cp` 去后缀使用；release 包内同名 `.example`。**模板的 `/imgs/` 块已内置图片扩展名白名单**（`if ($uri !~* \.(png|jpg|jpeg|gif|webp)$) return 404`），与后端 [`serveImages`](./internal/router/static.md) 形成前后端双重纵深防御；扩展 `storage.allowed_mime_types` 新增图片类型时，Nginx 正则与后端 `imageMimeExt` 都要同步。整体部署与反代约定详见 [`deploy.md`](../deploy.md)。

## 6. 常见排错

| 现象 | 原因 |
| --- | --- |
| 405 `Method Not Allowed` | 用了未注册的方法（如 PUT），中间件之前就被 Gin 拒 |
| 401 `CodeAPIKeyMissing` | 没带 `X-API-Key` |
| 401 `CodeAPIKeyInvalid` | 密钥格式不对 / 不存在 / 被吊销 |
| 403 `CodeForbidden` | 用只读密钥 POST |
| 413 `CodePayloadTooLarge` | 文件大于 `max_upload_size_mb` |
| 400 + "不支持的图片类型" | 内容嗅探结果不在 `allowed_mime_types`，伪造 Content-Type 无效 |
| 上传成功但 `width=0,height=0` | 是 webp/avif 等标准库未注册解码器的格式；不影响存储与 URL |
| 上传成功但 URL 拼接异常 | `public_base_url` 不应带尾斜杠；裸域名（无 `https://`）会被自动补 `https://`，留空则走 `/imgs/<rel>` 同域反代。若前端 src 形如 `/img.example.com/imgs/...`，说明 `public_base_url` 配了无协议裸域名且未升级到带补协议的版本 |
| 直接访问 `/imgs/<rel>` 返回 403 | 落盘文件权限/属主问题：新版已 `chmod 0644`；旧版本落盘为 0600，Nginx worker（如 `www`）作为 other 无法读取。历史文件需 `find <root_dir> -type f -exec chmod 644 {} \;` 批量补权限，并确认目录 0755 可被 Nginx worker 遍历 |
| 直接访问 `/imgs/<rel>` 返回 404 | 末段扩展名不在白名单（如 `.yaml`/`.db`/`.go`/无扩展名/目录）。这是 [`serveImages`](./internal/router/static.md) 的纵深防御：即便 `root_dir` 误配，也只放行图片扩展名 |
| 启动报 `storage.root_dir 不能是后端工作目录本身或其祖先` | `root_dir` 配成了 `.` / `..` / `/` / cwd 或其父级，会被 [`storage.NewSaver`](./internal/pkg/storage.md) 启动期 fail-fast 拒绝；改为 cwd 之外的专用目录或 cwd 之下的独立子目录（如 `data/imgs`） |

## 7. 不在本次范围

- `GET /api/v1/images` 对外列表 / 单图查询接口（语义待定，保持 501 占位）。后台列表已通过 `GET /api/v1/admin/images`（JWT）落地。
- 缩略图、EXIF 清理、防盗链、对象存储后端。
