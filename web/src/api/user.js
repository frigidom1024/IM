import http from './http.js'

export const authApi = {
  register: (data) => http.post('/auth/register', data),
  login: (data) => http.post('/auth/login', data)
}

export const userApi = {
  getInfo: (id) => http.get(`/user/info/${id}`),
  updateProfile: (data) => http.put('/user/profile', data)
}

export const friendApi = {
	getIncoming: () => http.get('/friend/incoming'),
	readNotice: id => http.post(`/friend/${id}/notice/read`),
  getList: () => http.get('/friend/list'),
  add: (data) => http.post('/friend/add', data),
  remove: (friendId) => http.delete(`/friend/${friendId}`)
}

export const groupApi = {
	getMyApplications: params => http.get('/group/applications/mine', { params }),
	getApplications: (id, params) => http.get(`/group/${id}/applications`, { params }),
	reviewApplication: (id, applicationId, decision, reason = '') => http.post(`/group/${id}/applications/${applicationId}/review`, { decision, reason }),
  getList: () => http.get('/group/list'),
  create: (data) => http.post('/group/create', data),
  join: (groupId) => http.post(`/group/${groupId}/join`),
  leave: (groupId) => http.delete(`/group/${groupId}/leave`),
  getMembers: (groupId, params) => http.get(`/group/${groupId}/members`, { params }),
  search: (q, params) => http.get('/group/search', { params: { ...params, q } }),
  invite: (groupId, userId) => http.post(`/group/${groupId}/invite`, { user_id: userId }),
  transferOwner: (groupId, userId) => http.post(`/group/${groupId}/transfer`, { user_id: userId }),
  updateAnnouncement: (groupId, announcement) => http.put(`/group/${groupId}/announcement`, { announcement }),
  updateMember: (groupId, userId, patch) => http.patch(`/group/${groupId}/members/${userId}`, patch),
  dissolve: groupId => http.delete(`/group/${groupId}`),
  getFiles: (groupId, params) => http.get(`/group/${groupId}/files`, { params }),
  removeMember: (groupId, userId) => http.delete(`/group/${groupId}/members/${userId}`)
}

export const conversationApi = {
  getMessages: (scope, params) => http.get(`/conversation/${encodeURIComponent(scope)}/messages`, { params }),
  markRead: scope => http.post(`/conversation/${encodeURIComponent(scope)}/read`),
  getUnreadCount: () => http.get('/conversation/unread'),
	open: scope => http.post(`/conversation/${encodeURIComponent(scope)}/open`),
  getList: params => http.get('/conversation', { params }),
  update: (scope, patch) => http.patch(`/conversation/${encodeURIComponent(scope)}`, patch)
}
