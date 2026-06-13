// 统一 fetch 封装：处理 JSON、错误、401 跳登录。
import { ElMessage } from 'element-plus'

export interface ApiError {
  code: string
  message: string
  unknown_items?: string[]
}

export class HttpError extends Error {
  status: number
  body: any
  apiError: ApiError | null
  constructor(status: number, body: any) {
    const ae: ApiError | null = body && body.error ? body.error : null
    super(ae?.message || `HTTP ${status}`)
    this.status = status
    this.body = body
    this.apiError = ae
  }
}

let onUnauthorized: (() => void) | null = null
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}

async function request<T>(
  method: string,
  path: string,
  body?: any,
  opts: { silent401?: boolean; raw?: boolean } = {}
): Promise<T> {
  const init: RequestInit = {
    method,
    credentials: 'same-origin',
    headers: {},
  }
  if (body !== undefined) {
    ;(init.headers as Record<string, string>)['Content-Type'] = 'application/json; charset=utf-8'
    init.body = typeof body === 'string' ? body : JSON.stringify(body)
  }
  const resp = await fetch(path, init)
  let parsed: any = null
  const text = await resp.text()
  if (text) {
    try {
      parsed = JSON.parse(text)
    } catch {
      parsed = text
    }
  }
  if (!resp.ok) {
    if (resp.status === 401 && !opts.silent401 && onUnauthorized) {
      onUnauthorized()
    }
    throw new HttpError(resp.status, parsed)
  }
  return parsed as T
}

export const api = {
  get: <T>(p: string, opts?: { silent401?: boolean }) => request<T>('GET', p, undefined, opts),
  post: <T>(p: string, body?: any, opts?: { silent401?: boolean }) =>
    request<T>('POST', p, body, opts),
  delete: <T>(p: string) => request<T>('DELETE', p),
}

export function showError(err: unknown, fallback = '请求失败') {
  if (err instanceof HttpError) {
    ElMessage.error(err.apiError?.message || err.message || fallback)
  } else if (err instanceof Error) {
    ElMessage.error(err.message || fallback)
  } else {
    ElMessage.error(fallback)
  }
}