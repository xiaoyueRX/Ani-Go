import { createHash } from 'node:crypto'
import { existsSync, mkdirSync, readdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { gzipSync } from 'node:zlib'
import { routes } from '../src/router/routes.ts'

const dist = resolve(import.meta.dirname, '../dist')
const manifestBytes = readFileSync(join(dist, 'route-manifest.json'))
const manifest = JSON.parse(manifestBytes)

function verifyRoutes(records) {
  for (const record of records) {
    const source = record.meta?.sourceModule
    if (source !== undefined) {
      const chunk = manifest[source]
      if (!chunk?.isDynamicEntry || chunk.src !== source || !existsSync(join(dist, chunk.file))) {
        throw new Error(`生产清单中缺少路由 ${record.path} 的页面：${source}`)
      }
    }
    if (record.children) verifyRoutes(record.children)
  }
}

const index = join(dist, 'index.html')
const html = readFileSync(index, 'utf8')
const entry = manifest['index.html']
if (!entry?.isEntry || !html.includes(`src="/${entry.file}"`) || !existsSync(join(dist, entry.file))) {
  throw new Error('网页入口与生产清单不一致')
}
verifyRoutes(routes)
const integrity = 'sha256-' + createHash('sha256').update(manifestBytes).digest('base64')
const meta = `<meta name="anigo-route-manifest" content="${integrity}">`
const clean = html.replace(/<meta name="anigo-route-manifest" content="[^"]*">\s*/g, '')
if (!clean.includes('</head>')) throw new Error('网页入口缺少 head')
writeFileSync(index, clean.replace('</head>', meta + '\n  </head>'))

const assets = join(dist, 'assets')
const encoded = join(assets, 'gzip')
mkdirSync(encoded, { recursive: true })

// 压缩 Vite 已落盘的产物，保留原文件及其内容哈希。
for (const entry of readdirSync(assets, { withFileTypes: true })) {
  if (!entry.isFile() || !/^.+-[\w-]{8}\.(css|js)$/.test(entry.name)) continue
  const source = readFileSync(join(assets, entry.name))
  const gzip = gzipSync(source, { level: 9 })
  if (gzip.length < source.length) writeFileSync(join(encoded, entry.name + '.gz'), gzip)
}
