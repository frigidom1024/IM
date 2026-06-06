<template>
  <div class="app">
    <aside class="sidebar">
      <div class="sidebar-top">
        <div class="my-avatar">{{ myName[0] || '?' }}</div>
        <div class="my-info">
          <div class="my-name">{{ myName }}</div>
          <div class="my-id">ID: {{ myId }}</div>
        </div>
        <div :class="['conn-dot', { on: wsConnected }]"></div>
      </div>

      <div class="sidebar-tabs">
        <button :class="['tab', { active: tab === 'friends' }]" @click="tab = 'friends'">消息</button>
        <button :class="['tab', { active: tab === 'groups' }]" @click="tab = 'groups'">群组</button>
      </div>

      <!-- 好友列表 -->
      <template v-if="tab === 'friends'">
        <div class="sidebar-actions">
          <input v-model="addFriendId" placeholder="添加好友 ID" @keydown.enter="handleAddFriend">
          <button @click="handleAddFriend">添加</button>
        </div>
        <div class="contacts">
          <div v-if="!friends.length" class="contacts-empty">暂无联系人</div>
          <div
            v-for="f in friends"
            :key="f.friend_id"
            :class="['contact', { active: currentChat?.id === f.friend_id && currentChat?.type === 'private' }]"
            @click="handleOpenChat(f.friend_id)"
          >
            <div class="avatar">{{ getFriendName(f.friend_id)[0] }}</div>
            <div class="contact-info">
              <div class="contact-name">{{ getFriendName(f.friend_id) }}</div>
              <div class="contact-sub">ID: {{ f.friend_id }}</div>
            </div>
            <div v-if="unreadMap[f.friend_id]" class="unread-badge">{{ unreadMap[f.friend_id] }}</div>
          </div>
        </div>
      </template>

      <!-- 群组列表 -->
      <template v-if="tab === 'groups'">
        <div class="sidebar-actions">
          <button @click="showCreateGroup = true">创建群组</button>
          <button @click="showJoinGroup = true">加入群组</button>
        </div>
        <div class="contacts">
          <div v-if="!groups.length" class="contacts-empty">暂无群组</div>
          <div
            v-for="g in groups"
            :key="g.id"
            :class="['contact', { active: currentChat?.id === g.id && currentChat?.type === 'group' }]"
            @click="handleOpenGroup(g)"
          >
            <div class="avatar">{{ g.name?.[0] || 'G' }}</div>
            <div class="contact-info">
              <div class="contact-name">{{ g.name }}</div>
              <div class="contact-sub">{{ g.member_count || 0 }} 人</div>
            </div>
            <div v-if="unreadMap['g'+g.id]" class="unread-badge">{{ unreadMap['g'+g.id] }}</div>
          </div>
        </div>
      </template>

      <!-- 创建群组弹窗 -->
      <div v-if="showCreateGroup" class="dialog-overlay" @click.self="showCreateGroup = false">
        <div class="dialog">
          <h3>创建群组</h3>
          <input v-model="groupName" placeholder="群组名称" @keydown.enter="handleCreateGroup">
          <input v-model="groupMax" type="number" placeholder="最大成员数(默认200)" @keydown.enter="handleCreateGroup">
          <div class="dialog-actions">
            <button @click="showCreateGroup = false">取消</button>
            <button class="primary" @click="handleCreateGroup">创建</button>
          </div>
        </div>
      </div>

      <!-- 加入群组弹窗 -->
      <div v-if="showJoinGroup" class="dialog-overlay" @click.self="showJoinGroup = false">
        <div class="dialog">
          <h3>加入群组</h3>
          <input v-model="joinGroupId" type="number" placeholder="群组 ID" @keydown.enter="handleJoinGroup">
          <div class="dialog-actions">
            <button @click="showJoinGroup = false">取消</button>
            <button class="primary" @click="handleJoinGroup">加入</button>
          </div>
        </div>
      </div>

      <button class="logout-btn" @click="handleLogout">退出登录</button>

      <div v-if="toastMsg" class="toast">{{ toastMsg }}</div>
    </aside>

    <main class="chat-area">
      <div v-if="!currentChat" class="chat-empty">选择联系人开始聊天</div>

      <template v-else>
        <div class="chat-header">
          <div class="avatar sm">{{ currentChat.name[0] }}</div>
          <span class="chat-header-name">{{ currentChat.name }}</span>
          <span class="chat-header-id">ID: {{ currentChat.id }}</span>
        </div>

        <div class="messages" ref="messagesEl">
          <div v-for="(msg, i) in messages" :key="i" :class="['msg', msg.isMine ? 'out' : 'in']">
            <template v-if="msg.msg_type === 'image'">
              <img class="msg-image" :src="getFileUrl(msg.content)" alt="图片" @click="previewImage(msg.content)">
            </template>
            <template v-else-if="msg.msg_type === 'file'">
              <div class="msg-file" @click="downloadFile(msg.content)">
                <span class="file-icon">&#128196;</span>
                <span class="file-name">{{ getFileName(msg.content) }}</span>
              </div>
            </template>
            <template v-else-if="msg.msg_type === 'voice'">
              <div class="msg-voice">
                <span>&#128266;</span>
                <span>{{ getVoiceDuration(msg.content) }}s</span>
              </div>
            </template>
            <template v-else>
              <div class="msg-content">{{ msg.content }}</div>
            </template>
            <div class="msg-time">{{ formatTime(msg.created_at) }}</div>
          </div>
        </div>

        <div class="chat-input">
          <input ref="fileInput" type="file" hidden @change="handleFileSelect" accept="image/*,.pdf,.zip,.doc,.docx,.mp3,.wav,.mp4">
          <button class="upload-btn" @click="$refs.fileInput.click()" title="发送文件">&#128206;</button>
          <textarea
            v-model="inputText"
            placeholder="输入消息..."
            rows="1"
            @keydown.enter.exact.prevent="send"
            @input="autoGrow"
          ></textarea>
          <button class="send-btn" @click="send">&#8593;</button>
        </div>
      </template>
    </main>
  </div>
</template>

<script setup>
import { ref, computed, nextTick, watch, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '../stores/user'
import { useChatStore } from '../stores/chat'
import { wsClient } from '../api/ws'
import { uploadApi } from '../api/upload'

const router = useRouter()
const userStore = useUserStore()
const chatStore = useChatStore()

const messagesEl = ref(null)
const fileInput = ref(null)
const inputText = ref('')
const addFriendId = ref('')
const wsConnected = ref(false)

const tab = ref('friends')
const showCreateGroup = ref(false)
const showJoinGroup = ref(false)
const groupName = ref('')
const groupMax = ref(200)
const joinGroupId = ref('')

const myId = computed(() => userStore.myId)
const myName = computed(() => userStore.myName)
const friends = computed(() => userStore.friends)
const friendMap = computed(() => userStore.friendMap)
const groups = computed(() => userStore.groups)
const currentChat = computed(() => chatStore.currentChat)
const messages = computed(() => chatStore.currentMessages)
const unreadMap = computed(() => chatStore.unreadMap)

onMounted(() => {
  wsClient.on('open', () => { wsConnected.value = true })
  wsClient.on('close', () => { wsConnected.value = false })
  wsClient.on('error', () => { wsConnected.value = false })
  wsClient.on('reconnect_failed', () => {
    wsConnected.value = false
    console.warn('WebSocket重连失败，请刷新页面重试')
  })
  if (wsClient.ws?.readyState === WebSocket.OPEN) wsConnected.value = true
})

function getFriendName(id) {
  const info = friendMap.value[id]
  return info?.nickname || info?.username || ('用户 ' + id)
}

function formatTime(t) {
  if (!t) return ''
  return new Date(t).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
}

function handleOpenChat(id) {
  const name = getFriendName(id)
  chatStore.openChat(id, name)
  nextTick(() => {
    if (messagesEl.value) messagesEl.value.scrollTop = messagesEl.value.scrollHeight
  })
}

function send() {
  const text = inputText.value.trim()
  if (!text || !currentChat.value) return
  chatStore.sendMessage(text)
  inputText.value = ''
  nextTick(() => {
    if (messagesEl.value) messagesEl.value.scrollTop = messagesEl.value.scrollHeight
  })
}

function autoGrow(e) {
  e.target.style.height = 'auto'
  e.target.style.height = Math.min(e.target.scrollHeight, 120) + 'px'
}

async function handleFileSelect(e) {
  const file = e.target.files?.[0]
  if (!file || !currentChat.value) return

  // 按 MIME 判断类型
  let fileType = 'file'
  if (file.type.startsWith('image/')) fileType = 'image'
  else if (file.type.startsWith('audio/')) fileType = 'voice'
  else if (file.type.startsWith('video/')) fileType = 'video'

  try {
    const meta = await uploadApi.upload(file, fileType)
    chatStore.sendMessage(meta, fileType)
  } catch (err) {
    console.error('[upload] failed:', err)
    alert('上传失败: ' + (err.message || ''))
  }
  // 重置 input 允许重复选择同一文件
  e.target.value = ''
}

async function handleAddFriend() {
  const id = parseInt(addFriendId.value.trim())
  if (!id) return
  try {
    await userStore.addFriend(id)
    addFriendId.value = ''
  } catch (e) { alert(e.message) }
}

function getFileUrl(raw) {
  try {
    const meta = JSON.parse(raw)
    return meta.thumbnail || meta.url
  } catch { return raw }
}
function getFileName(raw) {
  try { return JSON.parse(raw).name || '文件' } catch { return '文件' }
}
function getVoiceDuration(raw) {
  try { return JSON.parse(raw).duration || 0 } catch { return 0 }
}
function downloadFile(raw) {
  try {
    const meta = JSON.parse(raw)
    if (meta.url) window.open('/api/files/' + meta.url, '_blank')
  } catch {}
}
function previewImage(raw) {
  try {
    const meta = JSON.parse(raw)
    if (meta.url) window.open('/api/files/' + meta.url, '_blank')
  } catch {}
}

function handleOpenGroup(g) {
  chatStore.openChat(g.id, g.name, 'group')
  nextTick(() => {
    if (messagesEl.value) messagesEl.value.scrollTop = messagesEl.value.scrollHeight
  })
}

async function handleCreateGroup() {
  if (!groupName.value.trim()) return
  try {
    const group = await userStore.createGroup(groupName.value.trim(), groupMax.value || 200)
    showCreateGroup.value = false
    groupName.value = ''
    groupMax.value = 200
    tab.value = 'groups'
    showToast('群组创建成功')
  } catch (e) { alert(e.message) }
}

async function handleJoinGroup() {
  const id = parseInt(String(joinGroupId.value).trim())
  if (!id) return
  try {
    await userStore.joinGroup(id)
    showJoinGroup.value = false
    joinGroupId.value = ''
    tab.value = 'groups'
    showToast('已加入群组')
  } catch (e) { alert(e.message) }
}

let toastTimer = null
const toastMsg = ref('')
function showToast(msg) {
  toastMsg.value = msg
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toastMsg.value = '' }, 2500)
}

function handleLogout() {
  userStore.logout()
  router.push('/login')
}

watch(messages, () => {
  nextTick(() => {
    if (messagesEl.value) messagesEl.value.scrollTop = messagesEl.value.scrollHeight
  })
}, { deep: true })
</script>

<style scoped>
.app { display: flex; height: 100vh; }

.sidebar {
  width: 300px;
  background: var(--bg-2);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
}

.sidebar-top {
  padding: 18px 20px;
  border-bottom: 1px solid var(--border);
  display: flex; align-items: center; gap: 12px;
}

.my-avatar {
  width: 36px; height: 36px; border-radius: 50%;
  background: var(--accent); display: flex; align-items: center; justify-content: center;
  font-weight: 600; font-size: 14px; flex-shrink: 0;
}

.my-info { flex: 1; min-width: 0; }
.my-name { font-size: 14px; font-weight: 600; }
.my-id { font-family: 'IBM Plex Mono', monospace; font-size: 12px; color: var(--text-2); }

.conn-dot {
  width: 8px; height: 8px; border-radius: 50%;
  background: var(--text-3); flex-shrink: 0; transition: background .3s;
}
.conn-dot.on { background: var(--green); }

.sidebar-actions { padding: 10px 16px; display: flex; gap: 8px; }

.sidebar-actions input {
  flex: 1; padding: 8px 12px;
  background: var(--bg-3); border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--text);
  font-family: 'IBM Plex Mono', monospace; font-size: 12px;
}
.sidebar-actions input:focus { outline: none; border-color: var(--accent); }

.sidebar-tabs { display: flex; border-bottom: 1px solid var(--border); }
.sidebar-tabs .tab {
  flex: 1; padding: 10px 0; background: none; border: none;
  border-bottom: 2px solid transparent; color: var(--text-2);
  font-family: 'IBM Plex Mono', monospace; font-size: 13px; cursor: pointer;
  transition: all .2s;
}
.sidebar-tabs .tab.active { color: var(--text); border-bottom-color: var(--accent); }

.dialog-overlay {
  position: fixed; inset: 0; background: rgba(0,0,0,.6);
  display: flex; align-items: center; justify-content: center; z-index: 100;
}
.dialog {
  background: var(--bg-2); border: 1px solid var(--border); border-radius: 8px;
  padding: 24px; width: 340px; display: flex; flex-direction: column; gap: 12px;
}
.dialog h3 { font-size: 16px; font-weight: 600; color: var(--text); margin: 0; }
.dialog input {
  padding: 10px 12px; background: var(--bg-3); border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--text); font-size: 14px;
}
.dialog input:focus { outline: none; border-color: var(--accent); }
.dialog-actions { display: flex; gap: 8px; justify-content: flex-end; }
.dialog-actions button {
  padding: 8px 16px; border: 1px solid var(--border); border-radius: var(--radius);
  background: var(--bg-3); color: var(--text); font-size: 13px; cursor: pointer;
}
.dialog-actions button.primary { background: var(--accent); border-color: var(--accent); color: #fff; }
.dialog-actions button:hover { opacity: .85; }

.toast {
  position: fixed; bottom: 24px; left: 50%; transform: translateX(-50%);
  background: var(--bg-4); color: var(--text); padding: 10px 20px;
  border-radius: 8px; font-size: 13px; z-index: 200;
  animation: toastIn .2s ease;
}
@keyframes toastIn { from { opacity: 0; transform: translateX(-50%) translateY(10px); } }

.sidebar-actions button {
  padding: 8px 14px; background: var(--bg-4); border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--text); font-size: 12px;
  font-family: 'IBM Plex Mono', monospace; cursor: pointer; white-space: nowrap;
}
.sidebar-actions button:hover { background: var(--accent); border-color: var(--accent); }

.contacts { flex: 1; overflow-y: auto; }
.contacts-empty { padding: 24px; text-align: center; color: var(--text-3); font-size: 13px; }

.contact {
  padding: 14px 20px; display: flex; align-items: center; gap: 12px;
  cursor: pointer; border-bottom: 1px solid var(--border); transition: background .15s;
}
.contact:hover { background: var(--bg-3); }
.contact.active { background: color-mix(in srgb, var(--accent) 8%, var(--bg-4)); }

.avatar {
  width: 38px; height: 38px; border-radius: 50%;
  background: var(--bg-4); display: flex; align-items: center; justify-content: center;
  font-weight: 500; font-size: 14px; flex-shrink: 0;
}
.avatar.sm { width: 32px; height: 32px; font-size: 12px; }

.contact-info { flex: 1; min-width: 0; }
.contact-name { font-size: 14px; font-weight: 500; }
.contact-sub { font-family: 'IBM Plex Mono', monospace; font-size: 12px; color: var(--text-2); }

.unread-badge {
  background: var(--accent); color: #fff; font-size: 11px; font-weight: 600;
  padding: 2px 7px; border-radius: 10px; min-width: 20px; text-align: center;
}

.logout-btn {
  margin: 12px 16px; padding: 10px; background: var(--bg-3); border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--text-3); font-size: 13px; cursor: pointer;
}
.logout-btn:hover { color: var(--accent); border-color: var(--accent); }

.chat-area { flex: 1; display: flex; flex-direction: column; background: var(--bg); }

.chat-empty {
  flex: 1; display: flex; align-items: center; justify-content: center;
  color: var(--text-3); font-family: 'IBM Plex Mono', monospace; font-size: 13px;
}

.chat-header {
  padding: 14px 24px; border-bottom: 1px solid var(--border);
  display: flex; align-items: center; gap: 12px;
}
.chat-header-name { font-size: 15px; font-weight: 600; }
.chat-header-id {
  font-family: 'IBM Plex Mono', monospace; font-size: 11px; color: var(--text-3); margin-left: 8px;
}

.messages { flex: 1; padding: 20px 24px; overflow-y: auto; display: flex; flex-direction: column; gap: 6px; }

.msg {
  max-width: 65%; padding: 9px 14px; border-radius: 12px;
  font-size: 14px; line-height: 1.55; animation: msgIn .2s ease;
}
@keyframes msgIn { from { opacity: 0; transform: translateY(6px); } }

.msg.in { background: var(--bg-3); align-self: flex-start; border-bottom-left-radius: 4px; }
.msg.out { background: var(--accent); color: #fff; align-self: flex-end; border-bottom-right-radius: 4px; }

.msg-time {
  font-family: 'IBM Plex Mono', monospace; font-size: 10px;
  color: var(--text-3); margin-top: 3px;
}
.msg.out .msg-time { text-align: right; color: rgba(255,255,255,.5); }

.msg-image { max-width: 240px; max-height: 240px; border-radius: 8px; cursor: pointer; display: block; }
.msg-file { display: flex; align-items: center; gap: 8px; cursor: pointer; padding: 2px 0; }
.msg-file .file-icon { font-size: 24px; }
.msg-file .file-name { font-size: 13px; word-break: break-all; }
.msg-voice { display: flex; align-items: center; gap: 8px; font-size: 14px; }

.chat-input { padding: 12px 20px; border-top: 1px solid var(--border); display: flex; gap: 8px; align-items: flex-end; }

.chat-input textarea {
  flex: 1; padding: 10px 14px; background: var(--bg-2);
  border: 1px solid var(--border); border-radius: var(--radius);
  color: var(--text); font-size: 14px; resize: none;
  min-height: 42px; max-height: 120px; line-height: 1.5;
}
.chat-input textarea:focus { outline: none; border-color: var(--accent); }

.upload-btn, .send-btn {
  width: 42px; height: 42px; border: none; border-radius: var(--radius);
  font-size: 18px; cursor: pointer; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  transition: all .2s;
}
.upload-btn { background: var(--bg-3); color: var(--text-2); }
.upload-btn:hover { background: var(--bg-4); color: var(--text); }
.send-btn { background: var(--accent); color: #fff; }
.send-btn:hover { background: var(--accent-2); }
</style>
