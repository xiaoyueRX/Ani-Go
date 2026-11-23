import { mkdirSync, readdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { gzipSync } from 'node:zlib'

const assets = resolve(import.meta.dirname, '../dist/assets')
const encoded = join(assets, 'gzip')
mkdirSync(encoded, { recursive: true })

// 压缩 Vite 已落盘的产物，保留原文件及其内容哈希。
for (const entry of readdirSync(assets, { withFileTypes: true })) {
  if (!entry.isFile() || !/^.+-[\w-]{8}\.(css|js)$/.test(entry.name)) continue
  const source = readFileSync(join(assets, entry.name))
  const gzip = gzipSync(source, { level: 9 })
  if (gzip.length < source.length) writeFileSync(join(encoded, entry.name + '.gz'), gzip)
}
