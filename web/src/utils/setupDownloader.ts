export type DownloaderType = 'qbittorrent' | 'transmission' | 'aria2'
export interface DownloaderForm {
  host: string
  username: string
  password: string
  category: string
  secret: string
}

export function activeDownloader(settings: Record<string, string>): DownloaderType {
  const type = settings.DEFAULT_DOWNLOADER || settings.DOWNLOADER_DEFAULT || settings.DOWNLOADER_TYPE
  return type === 'aria2' || type === 'transmission' ? type : 'qbittorrent'
}

export function downloaderForms(settings: Record<string, string>): Record<DownloaderType, DownloaderForm> {
  const empty = { username: '', password: '', category: '', secret: '' }
  return {
    qbittorrent: {
      ...empty, host: settings.QB_HOST || 'http://localhost:8081', username: settings.QB_USER ?? 'admin',
      category: settings.QB_CATEGORY ?? 'ani-go',
    },
    transmission: { ...empty, host: settings.TR_HOST || 'http://localhost:9091', username: settings.TR_USER || '' },
    aria2: { ...empty, host: settings.ARIA2_HOST || 'http://localhost:6800/jsonrpc' },
  }
}

export function downloaderSettings(type: DownloaderType, form: DownloaderForm): Record<string, string> {
  const settings: Record<string, string> = { DEFAULT_DOWNLOADER: type }
  if (type === 'aria2') {
    settings.ARIA2_HOST = form.host
    if (form.secret) settings.ARIA2_SECRET = form.secret
  } else {
    const prefix = type === 'qbittorrent' ? 'QB' : 'TR'
    settings[`${prefix}_HOST`] = form.host
    settings[`${prefix}_USER`] = form.username
    if (form.password) settings[`${prefix}_PASS`] = form.password
    if (type === 'qbittorrent') settings.QB_CATEGORY = form.category
  }
  return settings
}
