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
        <button :class="['tab', { active: tab === 'messages' }]" @click="tab = 'messages'">消息 <span v-if="totalUnread" class="tab-count">{{ totalUnread }}</span></button>
        <button :class="['tab', { active: tab === 'friends' }]" @click="tab = 'friends'">好友 <span v-if="userStore.friendNoticeCount" class="tab-count">{{ userStore.friendNoticeCount }}</span></button>
        <button :class="['tab', { active: tab === 'groups' }]" @click="tab = 'groups'">群组 <span v-if="userStore.pendingGroupCount" class="tab-count">{{ userStore.pendingGroupCount }}</span></button>
      </div>

      <template v-if="tab === 'messages'">
        <div class="contacts">
          <div v-if="chatStore.conversationError" class="list-error" role="alert">{{ chatStore.conversationError }} <button @click="chatStore.loadConversations">重试</button></div>
          <div v-if="!conversations.length" class="contacts-empty">暂无会话<br>从好友或群组中打开聊天</div>
          <button v-for="c in conversations" :key="c.id" :class="['contact', 'conversation-row', { active: chatScope === c.id }]" @click="handleOpenConversation(c)">
            <div class="avatar"><img v-if="c.avatar" :src="c.avatar" alt=""><span v-else>{{ c.name?.[0] || '?' }}</span></div>
            <div class="contact-info"><div class="contact-name">{{ c.name }} <span class="type-label">{{ c.id.startsWith('group:') ? '群聊' : '私信' }}</span></div><div class="contact-sub">{{ c.draft ? '草稿：' + c.draft : messagePreview(c.last_message) }}</div></div>
            <div class="conversation-meta"><time>{{ formatTime(c.last_message_at) }}</time><span v-if="scopeUnread(c.id)" class="unread-badge">{{ scopeUnread(c.id) }}</span></div>
          </button>
        </div>
      </template>

      <!-- 好友关系与添加通知 -->
      <template v-if="tab === 'friends'">
        <div class="sidebar-actions">
          <input v-model="addFriendId" placeholder="添加好友 ID" @keydown.enter="handleAddFriend">
          <button @click="handleAddFriend">添加</button>
        </div>
        <div class="contacts">
          <h2 class="list-heading">收到的添加通知</h2>
          <div v-if="!userStore.incomingFriends.length" class="contacts-empty compact">暂无好友通知</div>
          <div v-for="notice in userStore.incomingFriends" :key="notice.user_id" class="relation-row">
            <div class="contact-info"><div class="contact-name">{{ notice.name }} <span v-if="notice.unread" class="notice-dot" aria-label="未读通知"></span></div><div class="contact-sub">{{ notice.added_by_me ? '已互加好友' : '对方添加了你' }}</div></div>
            <div class="row-actions"><button v-if="!notice.added_by_me" @click="addBack(notice.user_id)">添加好友</button><button v-if="notice.unread" @click="readFriendNotice(notice.user_id)">已读</button></div>
          </div>
          <h2 class="list-heading">我的好友</h2>
          <div v-if="!friends.length" class="contacts-empty">暂无联系人</div>
          <div
            v-for="f in friends"
            :key="f.friend_id"
            class="relation-row"
          >
            <div class="avatar">{{ getFriendName(f.friend_id)[0] }}</div>
            <div class="contact-info">
              <div class="contact-name">{{ getFriendName(f.friend_id) }}</div>
              <div class="contact-sub">{{ f.added_by_peer ? '互加好友' : '单向添加' }} · ID: {{ f.friend_id }}</div>
            </div>
            <div class="row-actions"><button @click="handleOpenChat(f.friend_id)">发消息</button><button class="danger" @click="removeFriend(f.friend_id)">删除</button></div>
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
            class="relation-row"
          >
            <div class="avatar">{{ g.name?.[0] || 'G' }}</div>
            <div class="contact-info">
              <div class="contact-name">{{ g.name }}</div>
              <div class="contact-sub">{{ g.member_count || 0 }} 人</div>
            </div>
            <div class="row-actions"><button @click="handleOpenGroup(g)">进入群聊</button><button @click="manageGroup(g)">管理{{ g.pending_count ? ' (' + g.pending_count + ')' : '' }}</button></div>
          </div>
        </div>
      </template>

      <div v-if="tab === 'groups'" class="application-list">
        <h2 class="list-heading">我的入群申请</h2>
        <div v-if="!userStore.myApplications.length" class="contacts-empty compact">暂无入群申请</div>
        <div v-for="application in userStore.myApplications" :key="application.id" class="application-row">
          <div class="contact-name">{{ application.group_name }}</div><div class="contact-sub">{{ applicationStatus(application.status) }} · {{ formatDate(application.created_at) }}</div>
          <div v-if="application.reason" class="contact-sub">{{ application.reason }}</div>
          <button v-if="canReapply(application)" @click="reapply(application.group_id)">重新申请</button>
        </div>
      </div>

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
          <h3>申请加入群组</h3>
          <input v-model="joinGroupId" type="number" placeholder="群组 ID" @keydown.enter="handleJoinGroup">
          <div class="dialog-actions">
            <button @click="showJoinGroup = false">取消</button>
            <button class="primary" @click="handleJoinGroup">提交申请</button>
          </div>
        </div>
      </div>

      <button class="logout-btn" @click="handleLogout">退出登录</button>

      <!-- 群成员弹窗 -->
      <div v-if="showMembers" class="dialog-overlay" @click.self="showMembers = false">
        <div class="dialog">
          <h3>群成员 ({{ members.length }})</h3>
          <div class="member-list">
            <div v-for="m in members" :key="m.user_id" class="member-item">
              <span class="member-name">{{ m.user_id === userStore.myId ? '我' : '用户' + m.user_id }}</span>
              <span v-if="m.role === 2" class="member-role">群主</span>
              <span v-else-if="m.role === 1" class="member-role">管理员</span>
              <button v-if="canRemoveMember(m)" class="member-remove" @click="handleRemoveMember(m.user_id)">移除</button>
            </div>
          </div>
          <div class="dialog-actions"><button @click="showMembers = false">关闭</button></div>
        </div>
      </div>

      <div v-if="toastMsg" class="toast">{{ toastMsg }}</div>
    </aside>

    <main class="chat-area">
      <div v-if="tab !== 'messages'" class="management-pane">
        <template v-if="tab === 'friends'"><h1>好友关系</h1><p>管理你添加的好友和收到的添加通知。单向添加可发送一条文本，互加后可以完整聊天。</p></template>
        <template v-else>
          <h1>{{ selectedGroup?.name || '群组管理' }}</h1>
          <p v-if="!selectedGroup">创建或申请加入群组，查看申请状态；选择群组的“管理”处理成员和入群申请。</p>
          <template v-else>
            <p>群 ID：{{ selectedGroup.id }} · {{ selectedGroup.member_count }} / {{ selectedGroup.max_members }} 人</p>
            <div class="management-actions"><button @click="handleOpenGroup(selectedGroup)">进入群聊</button><button v-if="selectedGroup.owner_id !== myId" class="danger" @click="handleLeaveGroup(selectedGroup.id)">退出群组</button><button v-else class="danger" @click="dissolveGroup">解散群组</button></div>
            <div v-if="selectedGroup.role >= 1" class="invite-row"><input v-model="inviteUserId" placeholder="邀请用户 ID" aria-label="邀请用户 ID"><button @click="inviteMember">邀请入群</button></div>
            <template v-if="selectedGroup.owner_id === myId">
              <h2>入群申请 <span class="type-label">{{ selectedGroup.pending_count || 0 }} 条待处理</span></h2>
              <p v-if="!groupApplications.length" class="contact-sub">暂无申请</p>
              <div v-for="application in groupApplications" :key="application.id" class="review-row">
                <div><strong>{{ application.applicant_name || '用户 ' + application.user_id }}</strong><div class="contact-sub">{{ applicationStatus(application.status) }} · {{ formatDate(application.created_at) }}<span v-if="application.reason"> · {{ application.reason }}</span></div></div>
                <div v-if="application.status === 'pending'" class="row-actions"><button :disabled="reviewBusy" @click="reviewApplication(application, 'approve')">通过</button><button :disabled="reviewBusy" @click="reviewApplication(application, 'reject')">拒绝</button></div>
              </div>
            </template>
            <h2>群成员</h2>
            <div v-for="member in members" :key="member.user_id" class="review-row">
              <div>{{ userStore.profiles[member.user_id]?.nickname || '用户 ' + member.user_id }} <span class="type-label">{{ ['成员', '管理员', '群主'][member.role] }}</span></div>
              <div class="row-actions" v-if="member.user_id !== myId && selectedGroup.role > member.role"><button @click="handleRemoveMember(member.user_id)">移除</button><button v-if="selectedGroup.owner_id === myId" @click="transferOwner(member.user_id)">转让群主</button></div>
            </div>
          </template>
        </template>
        <div v-if="userStore.relationError" role="alert" class="list-error">{{ userStore.relationError }} <button @click="userStore.refreshRelations">重试</button></div>
      </div>
      <div v-else-if="!currentChat" class="chat-empty">选择私信或群聊会话开始聊天</div>

      <template v-else>
        <div class="chat-header">
          <div class="avatar sm">{{ currentChat.name[0] }}</div>
          <span class="chat-header-name">{{ currentChat.name }}</span>
          <span class="chat-header-id">ID: {{ currentChat.id }}</span>
          <div class="chat-header-actions">
            <button v-if="currentChat.type === 'group'" class="header-btn" @click="showGroupMembers(currentChat.id)">成员</button>
            <button v-if="currentChat.type === 'group'" class="header-btn" @click="manageCurrentGroup">管理群组</button>
            <button v-if="currentChat.type === 'private' && !chatStore.currentConversation?.added_by_me" class="header-btn" @click="addBack(currentChat.id)">添加好友</button>
          </div>
        </div>

        <div class="messages" ref="messagesEl">
          <div v-for="(msg, i) in messages" :key="i" :class="['msg', msg.isMine ? 'out' : 'in']">
            <div v-if="!msg.isMine" class="msg-sender">{{ userStore.profiles[msg.from_id]?.nickname || msg.sender_name || '用户' + msg.from_id }}</div>
            <template v-if="msg.msg_type === 'image'">
              <button v-if="getMediaUrl(msg.content, true)" class="media-button" aria-label="预览图片" @click="previewImage(msg.content)">
                <img class="msg-image" :src="getMediaUrl(msg.content, true)" alt="图片">
              </button>
              <button v-else class="msg-file media-button" @click="loadMedia(msg.content, true)">{{ mediaError(msg.content, true) ? '图片加载失败，点击重试' : '加载图片' }}</button>
            </template>
            <template v-else-if="msg.msg_type === 'file'">
              <button class="msg-file media-button" @click="downloadFile(msg.content)">
                <span class="file-icon">&#128196;</span>
                <span class="file-name">{{ getFileName(msg.content) }}</span>
              </button>
            </template>
            <template v-else-if="msg.msg_type === 'voice'">
              <audio v-if="getMediaUrl(msg.content)" class="msg-audio" :src="getMediaUrl(msg.content)" controls preload="metadata"></audio>
              <button v-else class="msg-voice media-button" @click="loadMedia(msg.content)">
                <span aria-hidden="true">&#128266;</span>
                <span>{{ getVoiceDuration(msg.content) }}s · 播放语音</span>
              </button>
            </template>
            <template v-else-if="msg.msg_type === 'video'">
              <video v-if="getMediaUrl(msg.content)" class="msg-video" :src="getMediaUrl(msg.content)" :poster="parseMedia(msg.content)?.thumbnail ? getMediaUrl(msg.content, true) || undefined : undefined" controls preload="metadata"></video>
              <button v-else class="msg-file media-button" @click="loadMedia(msg.content)">查看视频 · {{ getFileName(msg.content) }}</button>
            </template>
            <template v-else>
              <div class="msg-content">{{ msg.content }}</div>
            </template>
            <div class="msg-time">{{ formatTime(msg.created_at) }} <span v-if="msg.isMine">{{ msg.send_status === 'sending' ? '发送中' : msg.send_status === 'failed' ? '发送失败：' + msg.send_error : '已发送' }}</span><button v-if="msg.send_status === 'failed'" class="retry-message" @click="chatStore.retryMessage(msg)">重试</button></div>
          </div>
        </div>

        <div class="composer">
          <div v-if="currentChat.type === 'private' && sendPolicy !== 'unrestricted'" class="send-policy" role="status">{{ privateHint }}</div>
          <div v-if="pendingAttachments.length" class="attachment-drafts" aria-label="待发送附件">
            <div v-for="item in pendingAttachments" :key="item.id" :class="['attachment-draft', { error: item.status === 'error', uploading: item.status === 'uploading' || item.status === 'processing' }]" :title="attachmentHint(item)">
              <button class="attachment-content" :disabled="item.status !== 'error'" :aria-label="attachmentHint(item)" @click="attachmentDrafts.retry(chatScope, item.id)">
                <img v-if="item.preview" :src="item.preview" class="attachment-preview" :alt="item.file.name">
                <svg v-else class="attachment-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M14 2H5v20h14V7zM14 2v5h5M8 12h8M8 16h6" /></svg>
              </button>
              <button class="attachment-remove" :title="'移除附件 ' + item.file.name" :aria-label="'移除附件 ' + item.file.name" @click="attachmentDrafts.remove(chatScope, item.id)">
                <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="m4 4 8 8M12 4l-8 8" /></svg>
              </button>
            </div>
          </div>
          <div class="chat-input">
            <input ref="fileInput" type="file" hidden multiple @change="handleFileSelect" accept="image/*,.pdf,.zip,.doc,.docx,.mp3,.wav,.mp4">
            <button class="upload-btn" :disabled="!canAttach" @click="$refs.fileInput.click()" title="添加附件" aria-label="添加附件"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="m8 12 6-6a3 3 0 0 1 4 4l-8 8a5 5 0 0 1-7-7l8-8a7 7 0 0 1 10 10l-8 8" /></svg></button>
            <textarea
              v-model="inputText"
              placeholder="输入消息..."
              rows="1"
              @keydown.enter.exact="handleComposerEnter"
              @input="autoGrow"
            ></textarea>
            <button class="send-btn" @click="send" :disabled="!canSend" :title="sendHint" :aria-label="sendHint"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="m6 12 6-6 6 6M12 6v13" /></svg></button>
          </div>
        </div>
      </template>
    </main>
  </div>
</template>

<script setup>
import { ref, computed, nextTick, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '../stores/user'
import { useChatStore } from '../stores/chat'
import { wsClient } from '../api/ws'
import { uploadApi } from '../api/upload'
import { createMediaCache } from '../api/media-cache'
import { createAttachmentDrafts } from '../composables/attachment-drafts'
import { groupApi } from '../api/user'

const router = useRouter()
const userStore = useUserStore()
const chatStore = useChatStore()

const messagesEl = ref(null)
const fileInput = ref(null)
const inputText = ref('')
const addFriendId = ref('')
const wsConnected = ref(false)
const toastMsg = ref('')
const mediaRevision = ref(0)
const mediaErrors = ref({})
const attachmentRevision = ref(0)
const attachmentDrafts = createAttachmentDrafts({
  upload: (file, type, options) => file.size > 8 * 1024 * 1024 ? uploadApi.uploadResumable(file, type, options) : uploadApi.upload(file, type, options),
  abortSession: id => uploadApi.abortSession(id),
  onChange: () => { attachmentRevision.value++ }
})
const mediaCache = createMediaCache({ onChange: () => { mediaRevision.value++ } })

const tab = ref('messages')
const selectedGroup = ref(null)
const groupApplications = ref([])
const inviteUserId = ref('')
const reviewBusy = ref(false)
const offListeners = []
const showCreateGroup = ref(false)
const showJoinGroup = ref(false)
const groupName = ref('')
const groupMax = ref(200)
const joinGroupId = ref('')
const showMembers = ref(false)
const members = ref([])
const currentGroupId = ref(null)

const myId = computed(() => userStore.myId)
const myName = computed(() => userStore.myName)
const friends = computed(() => userStore.friends)
const friendMap = computed(() => userStore.friendMap)
const groups = computed(() => userStore.groups)
const currentChat = computed(() => chatStore.currentChat)
const messages = computed(() => chatStore.currentMessages)
const unreadMap = computed(() => chatStore.unreadMap)
const conversations = computed(() => chatStore.conversations)
const totalUnread = computed(() => Object.values(unreadMap.value).reduce((sum, n) => sum + n, 0))
const sendPolicy = computed(() => currentChat.value?.type === 'group' ? 'unrestricted' : chatStore.currentConversation?.send_policy || 'unavailable')
const canAttach = computed(() => sendPolicy.value === 'unrestricted')
const privateHint = computed(() => !chatStore.currentConversation ? '正在加载发送权限…' : !chatStore.currentConversation.added_by_me ? '添加对方为好友后可发送文本；互加后可完整聊天。' : sendPolicy.value === 'text_once' ? '对方尚未添加你：可以发送一条文本，互加后可完整聊天。' : '已发送首次文本，等待对方添加好友后继续聊天。')
const chatScope = computed(() => currentChat.value ? currentChat.value.type + ':' + currentChat.value.id : '')
const pendingAttachments = computed(() => { attachmentRevision.value; return [...attachmentDrafts.list(chatScope.value)] })
const attachmentsReady = computed(() => pendingAttachments.value.every(item => item.status === 'ready'))
const canSend = computed(() => wsConnected.value && attachmentsReady.value && (sendPolicy.value === 'unrestricted' || (sendPolicy.value === 'text_once' && !chatStore.sendingCurrent && !pendingAttachments.value.length)) && !!(inputText.value.trim() || pendingAttachments.value.length))
const sendHint = computed(() => !wsConnected.value ? '连接中，请稍候' : !attachmentsReady.value ? '等待附件上传完成，失败后可重试或移除' : '发送消息')

function formatFileSize(size) {
  if (size < 1024) return size + ' B'
  if (size < 1024 * 1024) return (size / 1024).toFixed(1) + ' KB'
  return (size / (1024 * 1024)).toFixed(1) + ' MB'
}

function attachmentHint(item) {
  const status = item.status === 'ready' ? '待发送' : item.status === 'processing' ? '处理中' : item.status === 'error' ? item.error + '，点击图标重试' : '上传中 ' + item.progress + '%'
  return item.file.name + ' · ' + formatFileSize(item.file.size) + ' · ' + status
}

onMounted(() => {
  wsClient.on('open', () => { wsConnected.value = true })
  wsClient.on('close', () => { wsConnected.value = false })
  wsClient.on('error', reply => { if (!reply?.seq) wsConnected.value = false })
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
  tab.value = 'messages'
  const name = getFriendName(id)
  chatStore.openChat(id, name)
  nextTick(() => {
    if (messagesEl.value) messagesEl.value.scrollTop = messagesEl.value.scrollHeight
  })
}

function send() {
  if (!canSend.value || !currentChat.value) return
  const text = inputText.value.trim()
  if (text) chatStore.sendMessage(text)
  attachmentDrafts.send(chatScope.value, (meta, type) => chatStore.sendMessage(meta, type))
  inputText.value = ''
  chatStore.saveDraft(chatScope.value, '')
  nextTick(() => {
    if (messagesEl.value) messagesEl.value.scrollTop = messagesEl.value.scrollHeight
  })
}

function handleComposerEnter(event) {
  if (event.isComposing || event.keyCode === 229) return
  event.preventDefault()
  send()
}

function autoGrow(e) {
  e.target.style.height = 'auto'
  e.target.style.height = Math.min(e.target.scrollHeight, 120) + 'px'
  chatStore.saveDraft(chatScope.value, inputText.value)
}

function handleFileSelect(e) {
  const files = Array.from(e.target.files || [])
  e.target.value = ''
  if (!files.length || !currentChat.value || !canAttach.value) return
  attachmentDrafts.add(chatScope.value, files)
}

async function handleAddFriend() {
  const id = parseInt(addFriendId.value.trim())
  if (!id) return
  try {
    await userStore.addFriend(id)
    addFriendId.value = ''
    showToast('好友已添加')
  } catch (e) { alert(e.message) }
}

function parseMedia(raw) {
  if (raw && typeof raw === 'object') return raw
  try { return JSON.parse(raw) } catch { return { url: raw } }
}
function mediaPath(raw, thumbnail = false) {
  const meta = parseMedia(raw)
  return (thumbnail ? meta?.thumbnail || meta?.url : meta?.url) || ''
}
function getMediaUrl(raw, thumbnail = false) {
  mediaRevision.value // Refresh the template when cached object URLs change.
  return mediaCache.get(mediaPath(raw, thumbnail))
}
function mediaError(raw, thumbnail = false) {
  return mediaErrors.value[mediaPath(raw, thumbnail)]
}
function cancelled(error) {
  return error.name === 'AbortError' || error.code === 'ERR_CANCELED'
}
async function fetchMedia(raw, thumbnail = false) {
  const path = mediaPath(raw, thumbnail)
  if (!path) throw new Error('消息缺少文件地址')
  return mediaCache.load(path)
}
async function loadMedia(raw, thumbnail = false) {
  const path = mediaPath(raw, thumbnail)
  try {
    await fetchMedia(raw, thumbnail)
    const errors = { ...mediaErrors.value }
    delete errors[path]
    mediaErrors.value = errors
  } catch (error) {
    if (cancelled(error)) return
    mediaErrors.value = { ...mediaErrors.value, [path]: error.message }
    showToast('文件加载失败：' + error.message)
  }
}
function getFileName(raw) { return parseMedia(raw)?.name || '文件' }
function getVoiceDuration(raw) { return parseMedia(raw)?.duration || 0 }
async function downloadFile(raw) {
  try {
    const url = await fetchMedia(raw)
    const link = document.createElement('a')
    link.href = url
    link.download = getFileName(raw)
    document.body.appendChild(link)
    link.click()
    link.remove()
  } catch (error) { if (!cancelled(error)) showToast('下载失败：' + error.message) }
}
async function previewImage(raw) {
  // Open synchronously to retain the browser's user-gesture permission. Render
  // only an img element; do not navigate a message-provided blob as a document.
  const preview = window.open('about:blank', '_blank')
  if (preview) preview.opener = null
  try {
    const url = await fetchMedia(raw)
    if (!preview || preview.closed) return
    const image = preview.document.createElement('img')
    image.src = url
    image.alt = getFileName(raw)
    image.style.maxWidth = '100%'
    preview.document.title = getFileName(raw)
    preview.document.body.replaceChildren(image)
  } catch (error) {
    preview?.close()
    if (!cancelled(error)) showToast('图片加载失败：' + error.message)
  }
}

function handleStorageChange(event) {
  if (event.key === 'token' || event.key === null) {
    mediaCache.clear()
    attachmentDrafts.clear()
    mediaErrors.value = {}
  }
}
onMounted(() => window.addEventListener('storage', handleStorageChange))
let mediaToken = userStore.token
watch([() => userStore.token, () => messages.value.map(msg => [msg.msg_type, msg.content])], ([token]) => {
  if (token !== mediaToken) {
    mediaCache.clear()
    attachmentDrafts.clear()
    mediaErrors.value = {}
    mediaToken = token
  }
  const visible = messages.value.filter(msg => ['image', 'voice', 'video', 'file'].includes(msg.msg_type))
  mediaCache.retain(visible.flatMap(msg => [mediaPath(msg.content), mediaPath(msg.content, true)]).filter(Boolean))
  if (!token) return
  // Only image previews are eager. Large voice/video bodies load on demand.
  for (const msg of visible) if (msg.msg_type === 'image' || (msg.msg_type === 'video' && parseMedia(msg.content)?.thumbnail)) {
    const path = mediaPath(msg.content, true)
    if (path && !mediaErrors.value[path]) fetchMedia(msg.content, true).catch(error => {
      if (!cancelled(error)) mediaErrors.value = { ...mediaErrors.value, [path]: error.message }
    })
  }
}, { immediate: true, flush: 'sync' })

onBeforeUnmount(() => {
  offListeners.forEach(off => off())
  window.removeEventListener('focus', refreshAll)
  mediaCache.clear()
  attachmentDrafts.clear()
  window.removeEventListener('storage', handleStorageChange)
  clearTimeout(toastTimer)
})

function handleOpenGroup(g) {
  tab.value = 'messages'
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
    showToast('申请已提交，等待群主审核')
  } catch (e) { alert(e.message) }
}

let toastTimer = null
function showToast(msg) {
  toastMsg.value = msg
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toastMsg.value = '' }, 2500)
}

async function showGroupMembers(groupId) {
  try {
    members.value = await userStore.getGroupMembers(groupId)
    currentGroupId.value = groupId
    showMembers.value = true
  } catch (e) { alert(e.message) }
}

async function handleLeaveGroup(groupId) {
  if (!confirm('确定退出该群组？')) return
  try {
    await userStore.leaveGroup(groupId)
    chatStore.closeChat()
    await chatStore.loadConversations()
    showToast('已退出群组')
  } catch (e) { alert(e.message) }
}

async function handleRemoveMember(userId) {
  if (!confirm('确定移除该成员？')) return
  try {
    await userStore.removeMember(currentGroupId.value, userId)
    members.value = members.value.filter(m => m.user_id !== userId)
    showToast('已移除')
    if (selectedGroup.value) await refreshManagedGroup()
  } catch (e) { alert(e.message) }
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

function scopeUnread(scope) { const [kind, id] = scope.split(':'); return unreadMap.value[kind === 'group' ? 'g' + id : id] || 0 }
function messagePreview(msg) { if (!msg) return '暂无消息'; if (msg.is_revoked) return '消息已撤回'; return msg.msg_type === 'text' ? msg.content : ({ image: '[图片]', file: '[文件]', voice: '[语音]', video: '[视频]', mention: '[提及]' }[msg.msg_type] || '[消息]') }
function applicationStatus(status) { return { pending: '待审核', approved: '已通过', rejected: '已拒绝', closed: '已关闭' }[status] || status }
function canReapply(application) { return application.status === 'rejected' && userStore.myApplications.find(item => item.group_id === application.group_id)?.id === application.id }
function canRemoveMember(member) { const group = groups.value.find(item => item.id === currentGroupId.value); return member.user_id !== myId.value && group && group.role > member.role }
function formatDate(time) { return new Date(time).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) }
function handleOpenConversation(conversation) { const [kind, id] = conversation.id.split(':'); tab.value = 'messages'; chatStore.openChat(Number(id), conversation.name, kind) }
async function addBack(id) { try { await userStore.addFriend(id); await userStore.readFriendNotice(id); await chatStore.loadConversations(); showToast('好友已添加') } catch (error) { showToast(error.message) } }
async function readFriendNotice(id) { try { await userStore.readFriendNotice(id) } catch (error) { showToast(error.message) } }
async function removeFriend(id) { if (!confirm('删除该好友？聊天历史会保留。')) return; try { await userStore.removeFriend(id); await chatStore.loadConversations() } catch (error) { showToast(error.message) } }
async function reapply(id) { try { await userStore.joinGroup(id); showToast('申请已提交') } catch (error) { showToast(error.message) } }
async function manageGroup(group) {
  tab.value = 'groups'; selectedGroup.value = group; currentGroupId.value = group.id; groupApplications.value = []
  try {
    const rows = []; let offset = 0
    for (;;) { const res = await groupApi.getMembers(group.id, { limit: 100, offset }); if (res.code !== 0) throw new Error(res.message); rows.push(...(res.data || [])); if ((res.data || []).length < 100) break; offset += 100 }
    if (selectedGroup.value?.id !== group.id) return
    members.value = rows; rows.forEach(member => userStore.loadProfile(member.user_id))
    if (group.owner_id === myId.value) {
      const applications = []; let page = 0
      for (;;) { const res = await groupApi.getApplications(group.id, { limit: 100, offset: page }); if (res.code !== 0) throw new Error(res.message); applications.push(...(res.data || [])); if ((res.data || []).length < 100) break; page += 100 }
      if (selectedGroup.value?.id === group.id) groupApplications.value = applications
    }
  } catch (error) { showToast(error.message) }
}
function manageCurrentGroup() { const group = groups.value.find(g => g.id === currentChat.value?.id); if (group) manageGroup(group) }
async function refreshManagedGroup() { await userStore.loadGroups(); const group = groups.value.find(g => g.id === selectedGroup.value?.id); if (group) await manageGroup(group); else selectedGroup.value = null }
async function reviewApplication(application, decision) {
  const reason = decision === 'reject' ? prompt('拒绝理由（可留空）', '') : ''
  if (reason === null) return
  reviewBusy.value = true
  try { const res = await groupApi.reviewApplication(application.group_id, application.id, decision, reason); if (res.code !== 0) throw new Error(res.message); await refreshManagedGroup(); showToast(decision === 'approve' ? '申请已通过' : '申请已拒绝') } catch (error) { showToast(error.message) } finally { reviewBusy.value = false }
}
async function inviteMember() { const id = Number(inviteUserId.value); if (!id) return; try { const res = await groupApi.invite(selectedGroup.value.id, id); if (res.code !== 0) throw new Error(res.message); inviteUserId.value = ''; await refreshManagedGroup(); showToast('已邀请入群') } catch (error) { showToast(error.message) } }
async function transferOwner(id) { if (!confirm('将群主转让给该成员？')) return; try { const res = await groupApi.transferOwner(selectedGroup.value.id, id); if (res.code !== 0) throw new Error(res.message); await refreshManagedGroup() } catch (error) { showToast(error.message) } }
async function dissolveGroup() { if (!confirm('解散群组？此操作无法撤销。')) return; try { const res = await groupApi.dissolve(selectedGroup.value.id); if (res.code !== 0) throw new Error(res.message); selectedGroup.value = null; await userStore.refreshRelations(); await chatStore.loadConversations() } catch (error) { showToast(error.message) } }
function refreshAll() { userStore.refreshRelations(); chatStore.loadConversations() }
onMounted(() => { window.addEventListener('focus', refreshAll); offListeners.push(wsClient.on('group_changed', () => { if (selectedGroup.value) refreshManagedGroup() })); refreshAll() })
watch(tab, value => { if (value !== 'messages') chatStore.closeChat() })
watch(groups, rows => { if (selectedGroup.value) selectedGroup.value = rows.find(group => group.id === selectedGroup.value.id) || null })
watch(chatScope, (scope, previous) => { if (previous) chatStore.saveDraft(previous, inputText.value); inputText.value = conversations.value.find(item => item.id === scope)?.draft || '' })
watch(() => chatStore.currentConversation?.id, () => { if (!inputText.value) inputText.value = chatStore.currentConversation?.draft || '' })
</script>

<style scoped>
.app { display: flex; height: 100vh; }
.conversation-row { width: 100%; border: 0; background: transparent; color: inherit; font: inherit; text-align: left; }
.conversation-row .contact-name, .my-name { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.conversation-row .contact-sub, .relation-row .contact-sub, .application-row .contact-sub { color: var(--text-2); }
.conversation-meta { display: flex; flex-direction: column; align-items: flex-end; gap: 5px; }
.conversation-meta time { font-size: 11px; color: var(--text-2); }
.type-label { font-size: 11px; font-weight: 400; color: var(--text-2); margin-left: 5px; }
.tab-count { font-size: 11px; margin-left: 3px; color: var(--accent); }
.avatar img { width: 100%; height: 100%; object-fit: cover; border-radius: inherit; }
.relation-row { padding: 12px 16px; display: flex; flex-wrap: wrap; align-items: center; gap: 8px; border-bottom: 1px solid var(--border); }
.row-actions { display: flex; flex-wrap: wrap; gap: 6px; }
.row-actions button, .management-pane button, .application-row button, .list-error button { border: 1px solid var(--border); background: var(--bg-2); color: var(--text); padding: 6px 9px; font: inherit; font-size: 12px; cursor: pointer; border-radius: var(--radius); }
.row-actions button:hover, .management-pane button:hover { border-color: var(--accent); }
button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
button:disabled { cursor: not-allowed; opacity: .5; }
.danger { color: var(--accent) !important; }
.list-heading { margin: 18px 16px 8px; font-size: 12px; font-weight: 600; color: var(--text-2); }
.contacts-empty.compact { padding: 12px 16px; text-align: left; }
.notice-dot { display: inline-block; width: 6px; height: 6px; border-radius: 50%; background: var(--accent); vertical-align: middle; }
.application-list { max-height: 40vh; overflow-y: auto; border-top: 1px solid var(--border); }
.application-row { padding: 10px 16px; border-bottom: 1px solid var(--border); }
.application-row button { margin-top: 8px; }
.management-pane { padding: 28px; overflow-y: auto; }
.management-pane h1 { font-size: 20px; margin: 0 0 12px; }
.management-pane h2 { font-size: 15px; margin: 28px 0 10px; }
.management-pane p { font-size: 14px; color: var(--text-2); line-height: 1.6; max-width: 65ch; }
.management-actions, .invite-row { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 18px; }
.invite-row input { min-width: 0; background: var(--bg-2); color: var(--text); padding: 8px; border: 1px solid var(--border); border-radius: var(--radius); }
.review-row { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px; padding: 12px 0; border-bottom: 1px solid var(--border); font-size: 13px; }
.send-policy, .list-error { padding: 10px 16px; font-size: 12px; color: var(--text-2); line-height: 1.5; }
.retry-message { border: 0; background: transparent; color: inherit; text-decoration: underline; cursor: pointer; margin-left: 6px; }

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

.chat-header-actions { margin-left: auto; display: flex; gap: 6px; }

.member-list { max-height: 300px; overflow-y: auto; display: flex; flex-direction: column; gap: 4px; }
.member-item {
  display: flex; align-items: center; gap: 8px; padding: 8px 10px;
  border-radius: var(--radius); font-size: 13px;
}
.member-item:hover { background: var(--bg-3); }
.member-name { flex: 1; }
.member-role { font-size: 10px; color: var(--text-2); font-family: 'IBM Plex Mono', monospace; }

.toast {
  position: fixed; bottom: 24px; left: 50%; transform: translateX(-50%);
  background: var(--bg-4); color: var(--text); padding: 10px 20px;
  border-radius: 8px; font-size: 13px; z-index: 200;
  animation: toastIn .2s ease;
}
@keyframes toastIn { from { opacity: 0; transform: translateX(-50%) translateY(10px); } }

.sidebar-actions button { font-size: 12px; }

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

/* ===== 统一按钮 ===== */
.logout-btn, .sidebar-actions button, .dialog-actions button, .header-btn, .member-remove {
  padding: 8px 14px; background: var(--bg-4); border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--text); font-size: 12px;
  font-family: 'IBM Plex Mono', monospace; cursor: pointer; white-space: nowrap;
  transition: all .15s;
}
.logout-btn, .sidebar-actions button { padding: 8px 14px; }
.dialog-actions button { padding: 8px 16px; }
.header-btn { padding: 6px 12px; font-size: 11px; }
.member-remove { padding: 4px 10px; font-size: 11px; }

.logout-btn:hover, .sidebar-actions button:hover,
.dialog-actions button:hover, .header-btn:hover { border-color: var(--accent); color: var(--accent); }
.dialog-actions button.primary {
  background: var(--accent); border-color: var(--accent); color: #fff;
}
.dialog-actions button.primary:hover { background: var(--accent-2); }
.header-btn.danger:hover { border-color: var(--accent); color: var(--accent); }
.member-remove:hover { background: var(--accent); border-color: var(--accent); color: #fff; }

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

.msg-sender { font-size: 11px; color: var(--accent); margin-bottom: 2px; font-weight: 500; }
.msg-image { max-width: 240px; max-height: 240px; border-radius: 8px; cursor: pointer; display: block; }
.msg-file { display: flex; align-items: center; gap: 8px; cursor: pointer; padding: 2px 0; }
.msg-file .file-icon { font-size: 24px; }
.msg-file .file-name { font-size: 13px; word-break: break-all; }
.media-button { background: none; border: 0; color: inherit; font: inherit; text-align: left; cursor: pointer; padding: 0; }
.media-button:focus-visible { outline: 2px solid currentColor; outline-offset: 3px; }
.msg-video { display: block; max-width: 240px; max-height: 240px; border-radius: 8px; }
.msg-audio { display: block; max-width: 240px; }
.msg-voice { display: flex; align-items: center; gap: 8px; font-size: 14px; }

.composer { border-top: 1px solid var(--border); }
.attachment-drafts { padding: 12px 20px 0; display: flex; flex-wrap: wrap; gap: 14px; max-height: 180px; overflow-y: auto; }
.attachment-draft { position: relative; width: 64px; height: 64px; flex-shrink: 0; background: var(--bg-2); border: 1px solid var(--border); border-radius: var(--radius); }
.attachment-content { display: grid; place-items: center; width: 100%; height: 100%; padding: 0; border: 0; border-radius: inherit; overflow: hidden; background: none; color: var(--text-2); cursor: pointer; }
.attachment-content:disabled { cursor: default; }
.attachment-preview { width: 100%; height: 100%; object-fit: cover; }
.attachment-icon { width: 28px; height: 32px; }
.attachment-draft.uploading .attachment-content { opacity: .55; }
.attachment-draft.error { border-color: var(--accent); }
.attachment-draft.error .attachment-content { color: var(--accent); }
.attachment-remove { position: absolute; top: 0; right: 0; transform: translate(50%, -50%); z-index: 1; display: grid; place-items: center; width: 22px; height: 22px; padding: 0; border: 1px solid var(--border); border-radius: 50%; background: var(--bg-2); color: var(--text); cursor: pointer; }
.attachment-remove svg { width: 12px; height: 12px; }
.attachment-remove:hover { background: var(--bg-4); color: var(--accent); }
.attachment-remove:focus-visible, .attachment-content:focus-visible, .upload-btn:focus-visible, .send-btn:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.chat-input { padding: 12px 20px; display: flex; gap: 8px; align-items: flex-end; }
.chat-area { min-width: 0; }

.chat-input textarea {
  flex: 1; min-width: 0; padding: 10px 14px; background: var(--bg-2);
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
.upload-btn svg, .send-btn svg { width: 22px; height: 22px; }
.send-btn:disabled { background: var(--bg-3); color: var(--text-2); cursor: not-allowed; }
@media (max-width: 600px) {
  .sidebar { width: 136px; flex-shrink: 0; }
  .sidebar-top, .contact { padding: 12px 10px; gap: 8px; }
  .sidebar-tabs .tab { padding-inline: 3px; }
  .conversation-row { display: grid; grid-template-columns: minmax(0, 1fr); gap: 6px; }
  .conversation-row .avatar { display: none; }
  .conversation-row .contact-sub { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .conversation-meta { flex-direction: row; justify-content: space-between; }
  .management-pane { padding: 16px; }
  .relation-row { padding: 10px 8px; }
  .sidebar-actions { padding: 8px; flex-wrap: wrap; }
  .sidebar-actions input { min-width: 0; width: 100%; }
  .chat-header { padding: 12px 10px; gap: 8px; flex-wrap: wrap; }
  .chat-header-id { display: none; }
  .messages { padding: 16px 10px; }
  .attachment-drafts { padding: 12px 12px 0; }
  .chat-input { padding: 8px; gap: 6px; }
  .chat-input textarea { padding: 10px 8px; font-size: 16px; }
}
</style>
