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

      <div class="sidebar-actions">
        <input v-model="addFriendId" placeholder="添加好友 ID" @keydown.enter="handleAddFriend">
        <button @click="handleAddFriend">添加</button>
      </div>

      <div class="contacts">
        <div v-if="!friends.length" class="contacts-empty">暂无联系人</div>
        <div
          v-for="f in friends"
          :key="f.friend_id"
          :class="['contact', { active: currentChat?.id === f.friend_id }]"
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

      <button class="logout-btn" @click="handleLogout">退出登录</button>
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
            <div class="msg-content">{{ msg.content }}</div>
            <div class="msg-time">{{ formatTime(msg.created_at) }}</div>
          </div>
        </div>

        <div class="chat-input">
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

const router = useRouter()
const userStore = useUserStore()
const chatStore = useChatStore()

const messagesEl = ref(null)
const inputText = ref('')
const addFriendId = ref('')
const wsConnected = ref(false)

const myId = computed(() => userStore.myId)
const myName = computed(() => userStore.myName)
const friends = computed(() => userStore.friends)
const friendMap = computed(() => userStore.friendMap)
const currentChat = computed(() => chatStore.currentChat)
const messages = computed(() => chatStore.currentMessages)
const unreadMap = computed(() => chatStore.unreadMap)

onMounted(() => {
  wsClient.on('open', () => { wsConnected.value = true })
  wsClient.on('close', () => { wsConnected.value = false })
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

async function handleAddFriend() {
  const id = parseInt(addFriendId.value.trim())
  if (!id) return
  try {
    await userStore.addFriend(id)
    addFriendId.value = ''
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

.chat-input { padding: 16px 24px; border-top: 1px solid var(--border); display: flex; gap: 10px; }

.chat-input textarea {
  flex: 1; padding: 10px 14px; background: var(--bg-2);
  border: 1px solid var(--border); border-radius: var(--radius);
  color: var(--text); font-size: 14px; resize: none;
  min-height: 42px; max-height: 120px; line-height: 1.5;
}
.chat-input textarea:focus { outline: none; border-color: var(--accent); }

.send-btn {
  width: 42px; height: 42px; background: var(--accent); border: none;
  border-radius: var(--radius); color: #fff; font-size: 18px; cursor: pointer;
  display: flex; align-items: center; justify-content: center;
  transition: background .2s; flex-shrink: 0;
}
.send-btn:hover { background: var(--accent-2); }
</style>
