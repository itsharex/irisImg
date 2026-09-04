import type { FetchOptions } from 'ofetch'

/** 单张图片元信息，对应后端 model.Image。 */
export interface ImageItem {
  id: number
  filename: string
  stored_path: string
  url: string
  size: number
  mime_type: string
  width: number
  height: number
  hash: string
  created_at: string
  key_id?: number | null
}

/** 图片列表分页响应，对应 GET /admin/images 的 data。 */
export interface ImageListResponse {
  items: ImageItem[]
  total: number
  page: number
  page_size: number
}

export interface ListImagesParams {
  /** 指定密钥 ID 时只返回该密钥添加的图片；不传则返回全部。 */
  keyId?: number
  /** 时间排序方向，默认 asc（升序）。 */
  order?: 'asc' | 'desc'
  /** 页码，从 1 开始，默认 1。 */
  page?: number
  /** 每页条数，默认 24。 */
  pageSize?: number
}

/** 批量删除图片请求体（账号密码二次确认 + 待删除 ID 列表）。 */
export interface BatchDeleteImagesRequest {
  username: string
  password: string
  ids: number[]
}

/** 批量删除图片响应：deleted 为实际删除条数，ids 为实际被删除的图片 ID（不存在的 ID 静默跳过）。 */
export interface BatchDeleteImagesResult {
  deleted: number
  ids: number[]
}

/**
 * useImages 封装内容中心用到的图片列表请求、后台直传上传与批量删除。
 *
 * - 列表走后台 JWT 通道 GET /admin/images（由 useApi 自动附带 Authorization 头）。
 * - 上传走后台 JWT 通道 POST /admin/images，与对外 /images（API Key 鉴权）解耦；
 *   上传的图片不关联密钥（key_id 留空，即 admin 直传）。
 * - 批量删除走后台 JWT 通道 DELETE /admin/images，需账号密码二次确认。
 */
export function useImages() {
  const { get, post, api } = useApi()

  async function list(params: ListImagesParams = {}): Promise<ImageListResponse> {
    const query: Record<string, string> = {
      order: params.order ?? 'asc',
      page: String(params.page ?? 1),
      page_size: String(params.pageSize ?? 24),
    }
    if (params.keyId != null) {
      query.key_id = String(params.keyId)
    }
    return get<ImageListResponse>('/admin/images', { query } as FetchOptions)
  }

  /**
   * upload 经后台 JWT 通道上传一张图片（admin 直传，不关联密钥）。
   *
   * - 走 POST /admin/images，FormData 字段名固定 "file"（与后端 uploadFormField 一致）。
   * - ofetch 对 FormData body 自动设置 multipart/form-data 边界，切勿手动设 Content-Type。
   * - useApi 的 onRequest 自动附带 Authorization 头、onResponse 自动解包 data，
   *   返回值即新增的 ImageItem（key_id 为 null）。
   */
  async function upload(file: File): Promise<ImageItem> {
    const fd = new FormData()
    fd.append('file', file)
    return post<ImageItem>('/admin/images', fd)
  }

  /**
   * batchRemove 经后台 JWT 通道批量删除图片（物理文件 + 记录同删，不可恢复）。
   *
   * - 走 DELETE /admin/images，body 携带账号密码（后端二次确认）与待删除 ID 列表
   *   （后端限制非空、每项 > 0、上限 100）。
   * - 二次确认失败后端返回 403（而非 401），不会触发 useApi 的全局登出，
   *   调用方可在弹窗内联展示错误并让用户重试。
   * - 不存在的 ID 静默跳过：返回的 deleted / ids 均为「实际删除」口径。
   */
  async function batchRemove(params: BatchDeleteImagesRequest): Promise<BatchDeleteImagesResult> {
    return api<BatchDeleteImagesResult>('/admin/images', { method: 'DELETE', body: params })
  }

  return { list, upload, batchRemove }
}

/**
 * resolveImageUrl 把后端返回的图片 URL 解析成浏览器可加载的完整地址。
 *
 * - 后端 storage.public_base_url 为空时，url 形如 "/imgs/2026/07/<hash>.png"（相对路径），
 *   前端 SPA 跑在另一个端口（如 :3000），需拼上后端 origin 才能加载。
 * - 后端配了 public_base_url 时，url 已是 "https://..." 绝对地址，原样返回。
 */
export function resolveImageUrl(url: string | undefined | null): string {
  if (!url) return ''
  if (/^https?:\/\//i.test(url)) return url
  const { apiBase } = useRuntimeConfig().public
  // apiBase 形如 "http://localhost:8080/api/v1"，去掉 "/api/v1" 尾巴得到后端 origin。
  const origin = String(apiBase).replace(/\/api\/v1\/?$/i, '')
  return `${origin}${url.startsWith('/') ? '' : '/'}${url}`
}

/** formatBytes 把字节数格式化为带单位的可读字符串。 */
export function formatBytes(n: number): string {
  if (!n || n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 10 || i === 0 ? 0 : 1)} ${units[i]}`
}

/** formatDate 把 ISO 时间字符串格式化为「YYYY-MM-DD HH:mm」本地时间。 */
export function formatDate(s: string | undefined | null): string {
  if (!s) return '—'
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return s
  const pad = (x: number) => String(x).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}
