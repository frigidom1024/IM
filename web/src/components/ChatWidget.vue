<template>
  <div class="gw-root">
    <div class="gw-sidebar">
      <div class="gw-user">
        <div class="gw-avatar">{{ (myName || '?')[0] }}</div>
        <span>{{ myName }}</span>
      </div>
      <div class="gw-contacts">
        <div
          v-for="c in contacts"
          :key="c.id"
          :class="['gw-contact', { active: currentChat?.id === c.id }]"
          @click="openChat(c.id, c.name)"
        >
          <div class="gw-avatar sm">{{ (c.name || '?')[0] }}</div>
          <div class="gw-info">
            <div class="gw-name">{{ c.name }}</div>
            <div v-if="unreadMap[c.id]" class="gw-badge">{{ unreadMap[c.id] }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, watch } from 'vue'
import { useChat } from '../composables/useChat'

const props = defineProps({
  token: { type: String, required: true },
  contacts: { type: Array, default: () => [] }
})

const chat = useChat(props.token)
const { currentChat, unreadMap, openChat, myName } = chat

watch(() => props.token, (t) => {
  if (t) chat.connect()
})
</script>

<style scoped>
.gw-root {
  font-family: 'Noto Sans SC', sans-serif;
  background: var(--gw-bg, #0c0c0c);
  color: var(--gw-text, #e8e8e8);
  height: 100%; display: flex;
}
.gw-sidebar { width: 260px; border-right: 1px solid var(--gw-border, #2a2a2a); display: flex; flex-direction: column; background: var(--gw-bg-2, #141414); }
.gw-user { padding: 14px 16px; display: flex; align-items: center; gap: 10px; border-bottom: 1px solid var(--gw-border, #2a2a2a); font-size: 14px; font-weight: 600; }
.gw-avatar {
  width: 36px; height: 36px; border-radius: 50%; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  background: var(--gw-accent, #e85d3a); font-weight: 600; font-size: 14px;
}
.gw-avatar.sm { width: 32px; height: 32px; font-size: 12px; background: var(--gw-bg-4, #252525); }
.gw-contacts { flex: 1; overflow-y: auto; }
.gw-contact {
  padding: 12px 16px; display: flex; align-items: center; gap: 10px;
  cursor: pointer; border-bottom: 1px solid var(--gw-border, #2a2a2a);
  transition: background .15s;
}
.gw-contact:hover { background: var(--gw-bg-3, #1c1c1c); }
.gw-contact.active { background: color-mix(in srgb, var(--gw-accent, #e85d3a) 8%, var(--gw-bg-4, #252525)); }
.gw-info { flex: 1; display: flex; align-items: center; }
.gw-name { flex: 1; font-size: 14px; font-weight: 500; }
.gw-badge {
  background: var(--gw-accent, #e85d3a); color: #fff; font-size: 11px; font-weight: 600;
  padding: 2px 7px; border-radius: 10px; min-width: 20px; text-align: center;
}
</style>
