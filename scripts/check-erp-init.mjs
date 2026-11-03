import fs from 'node:fs'
import path from 'node:path'
import crypto from 'node:crypto'
import { fileURLToPath } from 'node:url'

const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const manifest = JSON.parse(fs.readFileSync(path.join(repo, 'sql/init/sources.sha256.json'), 'utf8'))
function hash(relative) {
  if (!/^(sql|internal)\/[a-zA-Z0-9_./-]+$/.test(relative) || relative.split('/').includes('..')) throw new Error(`Invalid manifest path: ${relative}`)
  const text = fs.readFileSync(path.join(repo, relative), 'utf8').replace(/\r\n/g, '\n')
  return crypto.createHash('sha256').update(text).digest('hex')
}
const stale = []
const declaredDDL = Object.keys(manifest.sources).filter(s => /^sql\/erp\/\d{3}_.+\.sql$/.test(s)).sort()
const actualDDL = fs.readdirSync(path.join(repo, 'sql/erp')).filter(s => /^\d{3}_.+\.sql$/.test(s)).map(s => `sql/erp/${s}`).sort()
if (JSON.stringify(actualDDL) !== JSON.stringify(declaredDDL)) stale.push('sql/erp: versioned SQL file list changed')
for (const [file, expected] of Object.entries(manifest.sources)) if (hash(file) !== expected) stale.push(file)
if (hash(manifest.sqlFile) !== manifest.sqlSha256) stale.push(manifest.sqlFile)
if (stale.length) {
  console.error('ERP initialization snapshot needs review; changed files:\n' + stale.map(s => `  ${s}`).join('\n'))
  process.exitCode = 1
} else {
  console.log(`ERP init snapshot verified: ${Object.keys(manifest.sources).length} source files, ${manifest.tables.length} tables, ${Object.keys(manifest.versions).length} migration records`)
}
