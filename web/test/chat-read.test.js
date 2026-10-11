import { test } from 'node:test'
import assert from 'node:assert/strict'
import { build } from 'esbuild'
import { fileURLToPath } from 'node:url'

// Bundle the real Pinia stores with the same resolver used by the frontend.
const bundle = await build({ stdin: { resolveDir: fileURLToPath(new URL('..', import.meta.url)), contents: `
export { createPinia, setActivePinia } from 'pinia';
export { useChatStore } from './src/stores/chat.js';
export { useUserStore } from './src/stores/user.js';
export { messageApi } from './src/api/user.js';
export { wsClient } from './src/api/ws.js';
` }, bundle: true, platform: 'node', format: 'esm', write: false, external: ['axios'] })
// A file URL provides Node's normal dependency resolution for axios.
import { writeFile, unlink } from 'node:fs/promises'
const bundlePath = new URL('./.chat-read-bundle.mjs', import.meta.url)
await writeFile(bundlePath, bundle.outputFiles[0].text)
let app
try { app = await import(bundlePath.href) } finally { await unlink(bundlePath) }
globalThis.localStorage = { getItem: () => null, removeItem() {} }

function setup() {
  app.setActivePinia(app.createPinia())
  const user = app.useUserStore(); user.user = { id: 1, nickname: 'Alice' }
  app.wsClient.handlers = {}
  app.wsClient.send = () => true
  return app.useChatStore()
}

test('opening a private chat persists read state before refresh instead of only hiding the badge', async () => {
  let persistedUnread = 1
  const calls = []
  const store = setup()
  app.messageApi.getPrivate = async () => ({ code: 0, data: [{ id: 321622086873387008, from_id: 2, content: 'unread', created_at: '2026-10-11T12:00:00Z' }] })
  app.messageApi.markRead = async body => { calls.push(body); persistedUnread = 0; return { code: 0 } }
  app.wsClient._emit('offline_notify', { data: { unread_list: [{ from_id: 2, count: persistedUnread }] } })
  await store.openChat(2, 'Bob')
  assert.equal(persistedUnread, 0, 'backend unread still present after opening chat')
  assert.deepEqual(calls, [{ from_id: 2 }])
  store.closeChat()
  app.wsClient._emit('offline_notify', { data: { unread_list: [{ from_id: 2, count: persistedUnread }] } })
  assert.equal(store.unreadMap['2'], 0, 'refresh restores unread badge')
})

test('visible incoming private/group messages are persisted as read without a real message ID', async () => {
  const calls = []
  const store = setup()
  app.messageApi.getGroup = async () => ({ code: 0, data: [] })
  app.messageApi.markRead = async body => { calls.push(body); return { code: 0 } }
  await store.openChat(3, 'Group', 'group')
  calls.length = 0
  app.wsClient._emit('msg', { data: { from_id: 2, group_id: 3, type: 'text', content: 'live group' } })
  await store.markRead(3, 'group')
  assert.ok(calls.some(body => body.group_id === 3))
  assert.equal(store.unreadMap.g3, 0)
  app.wsClient._emit('msg', { data: { from_id: 2, group_id: 0, type: 'text', content: 'other chat' } })
  assert.equal(store.unreadMap['2'], 1, 'closed chats must remain unread')
})

test('failed read requests retain unread state and retry after reconnect', async () => {
  const store = setup()
  app.messageApi.getPrivate = async () => ({ code: 0, data: [] })
  app.messageApi.markRead = async () => { throw new Error('offline') }
  app.wsClient._emit('offline_notify', { data: { unread_list: [{ from_id: 2, count: 4 }] } })
  await store.openChat(2, 'Bob')
  assert.equal(store.unreadMap['2'], 4, 'failure hides unread without acknowledgement')
  app.messageApi.markRead = async () => ({ code: 0 })
  app.wsClient._emit('open')
  await store.markRead(2)
  assert.equal(store.unreadMap['2'], 0)
})

test('messages arriving during a read request trigger a serialized follow-up acknowledgement', async () => {
  const store = setup()
  app.messageApi.getPrivate = async () => ({ code: 0, data: [] })
  const acknowledgements = []
  app.messageApi.markRead = () => new Promise(resolve => acknowledgements.push(resolve))
  const opening = store.openChat(2, 'Bob')
  await Promise.resolve()
  app.wsClient._emit('msg', { data: { from_id: 2, type: 'text', content: 'arrived during request' } })
  assert.equal(acknowledgements.length, 1, 'same scope must not issue parallel requests')
  acknowledgements[0]({ code: 0 })
  await Promise.resolve(); await Promise.resolve()
  assert.equal(acknowledgements.length, 2, 'new arrival needs another server acknowledgement')
  assert.equal(store.unreadMap['2'], 1, 'do not clear the new arrival before its acknowledgement')
  acknowledgements[1]({ code: 0 })
  await opening
  assert.equal(store.unreadMap['2'], 0)
})

test('switching chats while history loads does not clear the chat that is no longer visible', async () => {
  const store = setup()
  let finishFirst
  const calls = []
  app.messageApi.getPrivate = id => id === 2 ? new Promise(resolve => { finishFirst = resolve }) : Promise.resolve({ code: 0, data: [] })
  app.messageApi.markRead = async body => { calls.push(body); return { code: 0 } }
  const first = store.openChat(2, 'Bob')
  await store.openChat(3, 'Charlie')
  finishFirst({ code: 0, data: [] }); await first
  assert.deepEqual(calls, [{ from_id: 3 }])
})
