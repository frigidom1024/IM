import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import http from '../src/api/http.js'
import { createMediaCache } from '../src/api/media-cache.js'
import { uploadApi } from '../src/api/upload.js'

globalThis.localStorage = { getItem: () => 'user-token', removeItem() {} }

test('private file download includes Bearer and returns binary Blob', async () => {
 const server = createServer((request, response) => {
  assert.equal(request.url, '/api/file/asset-1/download')
  assert.equal(request.headers.authorization, 'Bearer user-token')
  response.setHeader('Content-Type', 'application/octet-stream')
  response.end(Buffer.from([0, 128, 255, 65]))
 })
 await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
 http.defaults.baseURL = `http://127.0.0.1:${server.address().port}/api`
 try {
  const blob = await uploadApi.download('/api/file/asset-1/download')
  assert.ok(blob instanceof Blob)
  assert.deepEqual([...new Uint8Array(await blob.arrayBuffer())], [0, 128, 255, 65])
  await assert.rejects(uploadApi.download('https://other.example/file'), /文件地址/)
 } finally { await new Promise(resolve => server.close(resolve)) }
})

test('media cache reuses blobs and revokes them when session is cleared', async () => {
 let requests = 0
 const server = createServer((request, response) => { requests++; response.setHeader('Content-Type', 'image/png'); response.end('image') })
 await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
 http.defaults.baseURL = `http://127.0.0.1:${server.address().port}/api`
 const cache = createMediaCache()
 try {
  const path = '/api/file/photo/thumbnail'
  const [first, second] = await Promise.all([cache.load(path), cache.load(path)])
  assert.equal(first, second)
  assert.equal(requests, 1)
  assert.equal(await (await fetch(first)).text(), 'image')
  cache.clear()
  assert.equal(cache.get(path), '')
  await assert.rejects(fetch(first))
 } finally { cache.clear(); await new Promise(resolve => server.close(resolve)) }
})

test('failed chunk retains its session and resumes the persisted offset without replay', async () => {
 let offset = 0
 let patches = 0
 const server = createServer(async (request, response) => {
  let body = ''
  for await (const chunk of request) body += chunk
  response.setHeader('Content-Type', 'application/json')
  if (request.url === '/api/upload/session' && request.method === 'POST') {
    const declaration = JSON.parse(body)
    assert.equal(declaration.group_id, 7)
    assert.equal(declaration.size, 6)
  }
  if (request.method === 'PATCH') {
    assert.equal(Number(request.headers['upload-offset']), offset)
    patches++
    if (patches === 1) {
      offset = 3
      response.writeHead(503)
      response.end(JSON.stringify({ code: 503, message: 'response lost' }))
      return
    }
    assert.equal(body, 'def')
    offset += body.length
  }
  if (request.url.endsWith('/complete')) {
    response.end(JSON.stringify({ code: 0, data: { size: 6, url: '/api/file/asset/download' } }))
    return
  }
  response.end(JSON.stringify({ code: 0, data: { id: 'session', size: 6, offset } }))
 })
 await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
 http.defaults.baseURL = `http://127.0.0.1:${server.address().port}/api`
 try {
  const file = new File(['abcdef'], 'data.txt', { type: 'text/plain' })
  let failed
  try { await uploadApi.uploadResumable(file, 'file', { groupId: 7 }) } catch (error) { failed = error }
  assert.equal(failed.sessionId, 'session')
  assert.equal(patches, 1)
  const progress = []
  const meta = await uploadApi.uploadResumable(file, 'file', { sessionId: failed.sessionId, onProgress: value => progress.push(value.loaded) })
  assert.equal(meta.size, 6)
  assert.equal(patches, 2)
  assert.deepEqual(progress, [3, 6])
 } finally { await new Promise(resolve => server.close(resolve)) }
})

test('clearing a cache aborts pending media and cannot recreate stale object URLs', async () => {
 let signalRequest
 const requestSeen = new Promise(resolve => { signalRequest = resolve })
 const server = createServer((request, response) => { signalRequest(); request.on('close', () => response.destroy()) })
 await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
 http.defaults.baseURL = `http://127.0.0.1:${server.address().port}/api`
 const cache = createMediaCache()
 try {
  const path = '/api/file/slow/download'
  const pending = cache.load(path)
  await requestSeen
  cache.clear()
  await assert.rejects(pending)
  assert.equal(cache.get(path), '')
 } finally { cache.clear(); server.closeAllConnections(); await new Promise(resolve => server.close(resolve)) }
})
