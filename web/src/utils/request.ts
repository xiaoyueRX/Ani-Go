import axios from 'axios'

declare module 'axios' {
  interface AxiosRequestConfig {
    confirmedSession?: string | null
  }
}

const request = axios.create({
  baseURL: '/api',
  timeout: 60000
})

request.interceptors.request.use(config => {
  const token = localStorage.getItem('token')
  if (config.confirmedSession !== undefined && config.confirmedSession !== token) {
    throw new axios.CanceledError('The confirmation session changed.')
  }
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

request.interceptors.response.use(
  response => response,
  error => {
    if (error.response?.status === 401) {
      const current = localStorage.getItem('token')
      if (!current || error.config?.headers?.Authorization === `Bearer ${current}`) {
        localStorage.removeItem('token')
        localStorage.removeItem('username')
        const { pathname, search, hash } = window.location
        if (pathname !== '/login') {
          window.location.replace('/login?redirect=' + encodeURIComponent(pathname + search + hash))
        }
      }
    }
    return Promise.reject(error)
  }
)

export default request
