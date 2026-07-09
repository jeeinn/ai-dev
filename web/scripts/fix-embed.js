/**
 * Fix Vite output for go:embed compatibility.
 * Go embed ignores files starting with _ or .
 */
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const distDir = path.resolve(__dirname, '../dist')
const assetsDir = path.join(distDir, 'assets')

if (!fs.existsSync(assetsDir)) {
  console.error('assets directory not found:', assetsDir)
  process.exit(1)
}

const renames = []
for (const name of fs.readdirSync(assetsDir)) {
  if (!name.startsWith('_')) continue
  const oldPath = path.join(assetsDir, name)
  if (!fs.statSync(oldPath).isFile()) continue
  const newName = name.slice(1)
  const newPath = path.join(assetsDir, newName)
  fs.renameSync(oldPath, newPath)
  renames.push({ from: name, to: newName })
}

if (renames.length === 0) {
  console.log('No underscore-prefixed assets to fix')
  process.exit(0)
}

function patchFile(filePath) {
  let content = fs.readFileSync(filePath, 'utf8')
  let changed = false
  for (const { from, to } of renames) {
    const next = content.replaceAll(from, to)
    if (next !== content) {
      content = next
      changed = true
    }
  }
  if (changed) {
    fs.writeFileSync(filePath, content)
  }
}

function walk(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const fullPath = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      walk(fullPath)
      continue
    }
    if (/\.(js|html|css)$/.test(entry.name)) {
      patchFile(fullPath)
    }
  }
}

walk(distDir)

for (const { from, to } of renames) {
  console.log(`Renamed: ${from} -> ${to}`)
}
console.log('Done fixing files for go:embed')
