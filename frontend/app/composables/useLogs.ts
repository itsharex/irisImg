import type { FetchOptions } from 'ofetch'

/** 日志级别，对应后端 model.Log.Level。 */
export type LogLevel = 'debug' | 'info' | 'warn' | 'error'

/** 单条日志，对应后端 model.Log。 */
export interface LogItem {
  id: number
  timestamp: string
  level: LogLevel
  event: string
  method?: string
  path?: string
  status?: number | null
  duration_ms?: number | null
  client_ip?: string
  request_id?: string
  api_key_id?: number | null
  username?: string
  message?: string
  created_at: string
}

/** 日志列表分页响应，对应 GET /admin/logs 的 data。 */
export interface LogListResponse {
  items: LogItem[]
  total: number
  page: number
  page_size: number
}

/** 直方图单日某来源（密钥 / 后台直传）的新增图片计数，仪表盘图片趋势 tooltip 用。 */
export interface KeyCount {
  /** 来源名称：密钥标签；后台 JWT 直传统一为 "admin"。 */
  name: string
  /** 该来源当日新增图片数。 */
  count: number
}

/** 直方图单日计数。 */
export interface HistogramBucket {
  date: string // YYYY-MM-DD
  count: number
  /** 当日按来源拆分的新增图片数；仅仪表盘图片趋势携带，日志直方图不携带（tooltip 不渲染来源明细）。 */
  keys?: KeyCount[]
}

/** 直方图响应，对应 GET /admin/logs/histogram 的 data。 */
export interface HistogramResponse {
  buckets: HistogramBucket[]
  total: number
}

/** 清理日志请求体（账号密码二次确认）。 */
export interface PurgeRequest {
  username: string
  password: string
  /** 清理范围：'all' 清空全部（默认，可省略）；'get' 仅清 info 级 GET 请求日志。 */
  scope?: 'all' | 'get'
}

export interface ListLogsParams {
  level?: string
  event?: string
  method?: string
  statusClass?: string
  keyword?: string
  /** RFC3339 时间字符串。 */
  start?: string
  end?: string
  page?: number
  pageSize?: number
}

/**
 * useLogs 封装日志中心用到的全部接口。
 *
 * 所有接口走后台 JWT 通道（由 useApi 自动附带 Authorization 头、自动解包 data）。
 * - 清理日志为敏感操作，需在请求体里携带账号密码做二次确认；
 *   后端密码校验失败返回 403（而非 401），避免触发 useApi 的全局登出。
 */
export function useLogs() {
  const { get, api } = useApi()

  async function list(params: ListLogsParams = {}): Promise<LogListResponse> {
    const query: Record<string, string> = {
      page: String(params.page ?? 1),
      page_size: String(params.pageSize ?? 50),
    }
    if (params.level) query.level = params.level
    if (params.event) query.event = params.event
    if (params.method) query.method = params.method
    if (params.statusClass) query.status_class = params.statusClass
    if (params.keyword) query.keyword = params.keyword
    if (params.start) query.start = params.start
    if (params.end) query.end = params.end
    return get<LogListResponse>('/admin/logs', { query } as FetchOptions)
  }

  async function histogram(): Promise<HistogramResponse> {
    return get<HistogramResponse>('/admin/logs/histogram')
  }

  /** 清理日志（scope 随 body 传递：'all' 全部 / 'get' 仅 info 级 GET），返回被删除的条数。 */
  async function purge(creds: PurgeRequest): Promise<{ deleted: number }> {
    return api<{ deleted: number }>('/admin/logs', { method: 'DELETE', body: creds })
  }

  return { list, histogram, purge }
}
