import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import { useUserStore } from './stores/user'

import Login from './views/Login.vue'
import Chat from './views/Chat.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: Login },
    { path: '/', component: Chat, meta: { auth: true } },
    { path: '/:pathMatch(.*)*', redirect: '/' }
  ]
})            

router.beforeEach((to) => {
  const userStore = useUserStore()
  if (to.meta.auth && !userStore.token) return '/login'
})

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')
