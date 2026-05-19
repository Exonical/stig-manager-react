const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '/api'

export interface AppInfo {
  version: string
  commit?: string
  buildDate?: string
}

export async function fetchAppInfo(): Promise<AppInfo> {
  const response = await fetch(`${API_BASE_URL}/v1/op/appinfo`, {
    headers: { Accept: 'application/json' },
  })
  if (!response.ok) {
    throw new Error(`appinfo: HTTP ${response.status}`)
  }
  return (await response.json()) as AppInfo
}
