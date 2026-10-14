import { test } from 'node:test'
import assert from 'node:assert/strict'
import { build } from 'esbuild'
import { fileURLToPath } from 'node:url'

// Bundle the real Pinia stores with the same resolver used by the frontend.
const bundle = await build({ stdin: { resolveDir: fileURLToPath(new URL('..', import.meta.url)), contents: `
export { createPinia, setActivePinia } from 'pinia';
export { createRenderer, nextTick } from 'vue';
export { useChat } from './src/composables/useChat.js';
export { useChatStore } from './src/stores/chat.js';
export { useConversationStore } from './src/stores/conversation.js';
export { useUserStore } from './src/stores/user.js';
export { conversationApi, friendApi, groupApi } from './src/api/user.js';
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
  app.wsClient.handlers = {}
  const user = app.useUserStore(); user.user = { id: 1, nickname: 'Alice' }
  app.wsClient.send = () => true
  app.conversationApi.open = async () => ({ code: 0 })
  app.conversationApi.getList = async () => ({ code: 0, data: [{ id: 'private:2' }, { id: 'private:3' }, { id: 'group:3' }] })
  app.conversationApi.getUnreadCount = async () => ({ code: 0, data: { count: 0, unread_list: [] } })
  app.friendApi.getList = app.friendApi.getIncoming = async () => ({ code: 0, data: [] })
  app.groupApi.getList = app.groupApi.getMyApplications = async () => ({ code: 0, data: [] })
  return app.useChatStore()
}

test('opening a private chat persists read state before refresh instead of only hiding the badge', async () => {
  let persistedUnread = 1
  const calls = []
  const store = setup()
  app.conversationApi.getMessages = async () => ({ code: 0, data: [{ id: 321622086873387008, from_id: 2, content: 'unread', created_at: '2026-10-11T12:00:00Z' }] })
  app.conversationApi.markRead = async body => { calls.push(body); persistedUnread = 0; return { code: 0 } }
  app.useConversationStore().unreadCounts = { 'private:2': persistedUnread }
  await store.openChat(2, 'Bob')
  assert.equal(persistedUnread, 0, 'backend unread still present after opening chat')
  assert.deepEqual(calls, ['private:2'])
  store.closeChat()
  app.useConversationStore().unreadCounts = { 'private:2': persistedUnread }
  assert.equal(store.unreadMap['2'], 0, 'refresh restores unread badge')
})

test('visible incoming private/group messages are persisted as whole-conversation reads', async () => {
  const calls = []
  const store = setup()
  app.conversationApi.getMessages = async () => ({ code: 0, data: [] })
  app.conversationApi.markRead = async body => { calls.push(body); return { code: 0 } }
  await store.openChat(3, 'Group', 'group')
  calls.length = 0
  app.wsClient._emit('msg', { data: { id: '501', conversation_id: 'group:3', sender_id: '2', type: 'text', content: 'live group' } })
  await store.markRead(3, 'group')
  assert.ok(calls.some(body => body === 'group:3'))
  assert.equal(store.unreadMap.g3, 0)
  app.wsClient._emit('msg', { data: { id: '502', conversation_id: 'private:2', sender_id: '2', type: 'text', content: 'other chat' } })
  assert.equal(store.unreadMap['2'], 1, 'closed chats must remain unread')
})

test('failed read requests retain unread state and retry after reconnect', async () => {
  const store = setup()
  app.conversationApi.getMessages = async () => ({ code: 0, data: [] })
  app.conversationApi.markRead = async () => { throw new Error('offline') }
  app.useConversationStore().unreadCounts = { 'private:2': 4 }
  await store.openChat(2, 'Bob')
  assert.equal(store.unreadMap['2'], 4, 'failure hides unread without acknowledgement')
  app.conversationApi.markRead = async () => ({ code: 0 })
  app.wsClient._emit('open')
  await store.markRead(2)
  assert.equal(store.unreadMap['2'], 0)
})

test('messages arriving during a read request trigger a serialized follow-up acknowledgement', async () => {
  const store = setup()
  app.conversationApi.getMessages = async () => ({ code: 0, data: [] })
  const acknowledgements = []
  app.conversationApi.markRead = () => new Promise(resolve => acknowledgements.push(resolve))
  const opening = store.openChat(2, 'Bob')
  for (let i = 0; i < 10 && !acknowledgements.length; i++) await Promise.resolve()
  app.wsClient._emit('msg', { data: { id: '503', conversation_id: 'private:2', sender_id: '2', type: 'text', content: 'arrived during request' } })
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
  app.conversationApi.getMessages = id => id === 'private:2' ? new Promise(resolve => { finishFirst = resolve }) : Promise.resolve({ code: 0, data: [] })
  app.conversationApi.markRead = async body => { calls.push(body); return { code: 0 } }
  const first = store.openChat(2, 'Bob')
  await store.openChat(3, 'Charlie')
  finishFirst({ code: 0, data: [] }); await first
  assert.deepEqual(calls, ['private:3'])
})

test('conversation store owns canonical counts while the chat store caches messages', async () => {
  const chat = setup(); const conversation = app.useConversationStore()
  app.conversationApi.getMessages = async () => ({ code: 0, data: [] })
  app.conversationApi.markRead = async () => ({ code: 0 })
  app.useConversationStore().unreadCounts = { 'private:2': 3 }
  assert.equal(conversation.unreadCounts['private:2'], 3)
  assert.equal(conversation.messagesCache, undefined)
  assert.strictEqual(chat.unreadMap, conversation.unreadMap)
  await chat.openChat(2, 'Bob')
  assert.equal(conversation.unreadCounts['private:2'], 0)
})

test('incoming messages during history load remain unread until history succeeds', async () => {
  const chat = setup(); let finishHistory; let reads = 0
  app.conversationApi.getMessages = () => new Promise(resolve => { finishHistory = resolve })
  app.conversationApi.markRead = async () => { reads++; return { code: 0 } }
  const opening = chat.openChat(2, 'Bob')
  for (let i = 0; i < 20 && !finishHistory; i++) await Promise.resolve()
  app.wsClient._emit('msg', { data: { id: '503', conversation_id: 'private:2', sender_id: '2', type: 'text', content: 'during history' } })
  assert.equal(reads, 0)
  assert.equal(chat.unreadMap['2'], 1)
  finishHistory({ code: 0, data: [] }); await opening
  assert.equal(reads, 1)
  assert.equal(chat.unreadMap['2'], 0)
})

test('core useChat keeps conversation and message state reactive across replacements', async () => {
  const store = setup(); let chat
  app.wsClient.connect = () => {}; app.wsClient.disconnect = () => {}
  app.conversationApi.getMessages = async () => ({ code: 0, data: [] })
  app.conversationApi.markRead = async () => ({ code: 0 })
  const renderer = app.createRenderer({ createElement: () => ({}), createText: () => ({}), createComment: () => ({}), insert() {}, remove() {}, setText() {}, setElementText() {}, parentNode: () => null, nextSibling: () => null, patchProp() {} })
  const component = renderer.createApp({ setup() { chat = app.useChat('token'); return () => null } })
  component.mount({})
  try {
    app.useConversationStore().unreadCounts = { 'private:2': 3 }
    assert.equal(chat.unreadMap.value['2'], 3)
    await chat.openChat(2, 'Bob')
    assert.equal(chat.currentChat.value.id, 2)
    app.wsClient._emit('msg', { data: { id: '9007199254740993', conversation_id: 'private:2', sender_id: '2', type: 'text', content: 'live' } })
    await store.markRead(2)
    assert.equal(chat.messages.value.length, 1)
    assert.equal(chat.messages.value[0].id, '9007199254740993')
    assert.equal(chat.unreadMap.value['2'], 0)
  } finally { component.unmount() }
})

test("a late history response cannot restore another account's message cache", async () => {
  const store = setup(); const user = app.useUserStore(); let finishHistory
  app.conversationApi.getMessages = () => new Promise(resolve => { finishHistory = resolve })
  app.conversationApi.markRead = async () => ({ code: 0 })
  const opening = store.openChat(2, 'Bob')
  for (let i = 0; i < 20 && !finishHistory; i++) await Promise.resolve()
  user.user = { id: 3, nickname: 'Other' }; await app.nextTick()
  finishHistory({ code: 0, data: [{ id: '9', from_id: 2, content: 'old account' }] }); await opening
  assert.deepEqual(store.messagesCache, {})
  assert.equal(store.currentChat, null)
})

test('startup and reconnect actively pull unread and relation notices through HTTP', async () => {
  const chat = setup(); const user = app.useUserStore(); let unread = 3; let notices = 0; let applications = 0
  app.conversationApi.getUnreadCount = async () => ({ code: 0, data: { count: unread, unread_list: unread ? [{ id: 'private:2', unread_count: unread }] : [] } })
  app.friendApi.getIncoming = async () => { notices++; return { code: 0, data: [{ user_id: 2, unread: true }] } }
  app.groupApi.getMyApplications = async () => { applications++; return { code: 0, data: [{ id: 7, status: 'approved' }] } }
  user.token = 'auth'; await app.nextTick(); await chat.loadConversations()
  assert.equal(chat.unreadMap['2'], 3)
  assert.equal(app.wsClient.handlers.offline_notify, undefined)
  unread = 5; app.wsClient._emit('open'); await chat.loadConversations()
  assert.equal(chat.unreadMap['2'], 5)
  assert.ok(notices > 0 && applications > 0)
  assert.equal(user.friendNoticeCount, 1)
})

test('an arrival during an HTTP snapshot schedules another pull and duplicate delivery is ignored', async () => {
  const chat = setup(); const user = app.useUserStore(); let finishFirst; let calls = 0
  app.conversationApi.getUnreadCount = () => {
    calls++
    return calls === 1 ? new Promise(resolve => { finishFirst = resolve }) : Promise.resolve({ code: 0, data: { unread_list: [{ id: 'private:2', unread_count: 1 }] } })
  }
  user.token = 'auth'; await app.nextTick()
  for (let i = 0; i < 30 && !finishFirst; i++) await Promise.resolve()
  const frame = { data: { id: '9007199254740993', conversation_id: 'private:2', sender_id: '2', type: 'text', content: 'during pull' } }
  app.wsClient._emit('msg', frame)
  finishFirst({ code: 0, data: { unread_list: [] } }); await chat.loadConversations()
  assert.ok(calls >= 2)
  assert.equal(chat.unreadMap['2'], 1)
  app.wsClient._emit('msg', frame)
  assert.equal(chat.messagesCache['2'].length, 1)
  assert.equal(chat.unreadMap['2'], 1)
})

test('reconnect reloads missed history before marking it read and preserves exact ordering', async () => {
  const chat = setup(); let reads = 0; let finishHistory
  app.conversationApi.getMessages = async () => ({ code: 0, data: [] })
  app.conversationApi.markRead = async () => { reads++; return { code: 0 } }
  await chat.openChat(2, 'Bob'); reads = 0
  app.conversationApi.getMessages = () => new Promise(resolve => { finishHistory = resolve })
  app.useConversationStore().unreadCounts = { 'private:2': 2 }
  app.wsClient._emit('close'); app.wsClient._emit('open')
  for (let i = 0; i < 30 && !finishHistory; i++) await Promise.resolve()
  assert.equal(reads, 0, 'reconnect cleared unseen messages before history loaded')
  assert.equal(chat.unreadMap['2'], 2)
  finishHistory({ code: 0, data: [
    { id_string: '9007199254740993', from_id: 2, content: 'second', created_at: '2026-10-12T12:00:00Z' },
    { id_string: '9007199254740992', from_id: 2, content: 'first', created_at: '2026-10-12T12:00:00Z' }
  ] })
  for (let i = 0; i < 30 && chat.unreadMap['2'] !== 0; i++) await Promise.resolve()
  assert.equal(reads, 1)
  assert.deepEqual(chat.currentMessages.map(item => item.content), ['first', 'second'])
  assert.equal(chat.unreadMap['2'], 0)
})

test('a hidden page keeps incoming messages unread until foreground history is loaded', async () => {
  const previous = globalThis.document; const handlers = new Set()
  globalThis.document = { visibilityState: 'hidden', addEventListener: (_, fn) => handlers.add(fn), removeEventListener: (_, fn) => handlers.delete(fn) }
  const chat = setup(); let reads = 0
  app.conversationApi.getMessages = async () => ({ code: 0, data: [] })
  app.conversationApi.markRead = async () => { reads++; return { code: 0 } }
  try {
    await chat.openChat(2, 'Bob')
    app.wsClient._emit('msg', { data: { id: '604', conversation_id: 'private:2', sender_id: '2', type: 'text', content: 'hidden' } })
    assert.equal(reads, 0)
    assert.equal(chat.unreadMap['2'], 1)
    document.visibilityState = 'visible'; for (const callback of handlers) callback()
    for (let i = 0; i < 30 && chat.unreadMap['2'] !== 0; i++) await Promise.resolve()
    assert.equal(reads, 1)
    assert.equal(chat.unreadMap['2'], 0)
  } finally {
    chat.$dispose(); app.useConversationStore().$dispose(); app.useUserStore().$dispose()
    if (previous === undefined) delete globalThis.document; else globalThis.document = previous
  }
})
