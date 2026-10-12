package handler

import (
	"gochat/internal/logic/model"
	"gochat/internal/pkg/mysql"
)

// Notify is best-effort invalidation after a committed mutation. Database
// queries remain authoritative and recover missed notifications on reconnect.
type Notify func(command string, data any, users ...int64)

func (n Notify) send(command string, data any, users ...int64) {
	if n != nil {
		n(command, data, users...)
	}
}
func (h *GroupHandler) notifyGroup(id int64, extra ...int64) {
	if h.notify == nil {
		return
	}
	var members []model.GroupMember
	mysql.Client.Where("group_id=?", id).Find(&members)
	for _, m := range members {
		extra = append(extra, m.UserID)
	}
	var pending []model.GroupApplication
	mysql.Client.Where("group_id=? AND status='pending'", id).Find(&pending)
	for _, a := range pending {
		extra = append(extra, a.UserID)
	}
	h.notify.send("group_changed", map[string]any{"group_id": id}, extra...)
}
