import type { Router } from 'vue-router'

export function loginDestination(router: Router, redirect: unknown): string {
  if (typeof redirect !== 'string' || !redirect.startsWith('/') || redirect.startsWith('//') || redirect.includes('\\')) return '/'
  const target = router.resolve(redirect)
  return target.name && target.name !== 'login' ? target.fullPath : '/'
}
