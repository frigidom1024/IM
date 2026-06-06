import http from './http'

export const authApi = {
  register: (data) => http.post('/auth/register', data),
  login: (data) => http.post('/auth/login', data)
}

export const userApi = {
  getInfo: (id) => http.get(`/user/info/${id}`),
  updateProfile: (data) => http.put('/user/profile', data)
}

export const friendApi = {
  getList: () => http.get('/friend/list'),
  add: (data) => http.post('/friend/add', data),
  remove: (friendId) => http.delete(`/friend/${friendId}`)
}

export const groupApi = {
  getList: () => http.get('/group/list'),
  create: (data) => http.post('/group/create', data),
  join: (groupId) => http.post(`/group/${groupId}/join`),
  leave: (groupId) => http.delete(`/group/${groupId}/leave`),
  getMembers: (groupId) => http.get(`/group/${groupId}/members`),
  removeMember: (groupId, userId) => http.delete(`/group/${groupId}/members/${userId}`)
}

export const messageApi = {
  getPrivate: (userId) => http.get(`/message/private/${userId}`),
  getGroup: (groupId) => http.get(`/message/group/${groupId}`),
  markRead: (data) => http.post('/message/read', data),
  getUnreadCount: () => http.get('/message/unread')
}
