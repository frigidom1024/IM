package handler

import (
	"errors"
	"github.com/gin-gonic/gin"
	"gochat/internal/logic/model"
	"gochat/internal/logic/service"
	apierrors "gochat/internal/pkg/errors"
	"gochat/internal/pkg/mysql"
	"gorm.io/gorm"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func mediaError(c *gin.Context, err error) {
	status, code, message := 500, apierrors.CodeServerError, "文件操作失败"
	switch {
	case errors.Is(err, service.ErrMediaForbidden):
		status, code, message = 403, 40300, err.Error()
	case errors.Is(err, service.ErrUploadConflict):
		status, code, message = 409, 40900, err.Error()
	case errors.Is(err, service.ErrMediaExpired):
		status, code, message = 410, 41000, err.Error()
	case errors.Is(err, gorm.ErrRecordNotFound):
		status, code, message = 404, 40400, "文件或上传不存在"
	}
	c.JSON(status, gin.H{"code": code, "message": message, "data": nil})
}

func validMedia(kind, contentType string, size int64) bool {
	maxSize, ok := maxSizeMap[kind]
	if !ok || size <= 0 || size > maxSize {
		return false
	}
	for _, allowed := range allowedMIME[kind] {
		if allowed == contentType {
			return true
		}
	}
	return false
}

func (h *UploadHandler) mediaService() *service.MediaService {
	return &service.MediaService{Store: h.minioClient}
}

func (h *UploadHandler) CreateSession(c *gin.Context) {
	var req struct {
		Name         string `json:"name"`
		Type         string `json:"type"`
		MIME         string `json:"mime"`
		Size         int64  `json:"size"`
		GroupID      int64  `json:"group_id"`
		ExpiresHours int    `json:"expires_hours"`
	}
	if c.ShouldBindJSON(&req) != nil || req.Name == "" || len(req.Name) > 255 || req.GroupID < 0 || req.ExpiresHours < 0 || req.ExpiresHours > 24*365 || !validMedia(req.Type, req.MIME, req.Size) {
		apierrors.RespondBadRequest(c, "无效的上传声明")
		return
	}
	session := &model.UploadSession{UserID: c.GetInt64("userID"), Name: filepath.Base(strings.ReplaceAll(req.Name, "\\", "/")), Type: req.Type, MIME: req.MIME, Size: req.Size, GroupID: req.GroupID}
	if req.ExpiresHours > 0 {
		expiry := time.Now().Add(time.Duration(req.ExpiresHours) * time.Hour)
		session.AssetExpiresAt = &expiry
	}
	if err := h.mediaService().CreateSession(c.Request.Context(), session); err != nil {
		mediaError(c, err)
		return
	}
	apierrors.RespondOK(c, session)
}

func (h *UploadHandler) GetSession(c *gin.Context) {
	session, err := h.mediaService().Session(c.Request.Context(), c.Param("uploadId"), c.GetInt64("userID"))
	if err != nil {
		mediaError(c, err)
		return
	}
	apierrors.RespondOK(c, session)
}

func (h *UploadHandler) AppendSession(c *gin.Context) {
	offset, err := strconv.ParseInt(c.GetHeader("Upload-Offset"), 10, 64)
	if err != nil || offset < 0 || c.Request.ContentLength > 8<<20 {
		apierrors.RespondBadRequest(c, "无效的上传偏移或分块大小，最多8MiB")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, (8<<20)+1)
	if err = h.mediaService().Append(c.Request.Context(), c.Param("uploadId"), c.GetInt64("userID"), offset, c.Request.Body); err != nil {
		mediaError(c, err)
		return
	}
	h.GetSession(c)
}

func (h *UploadHandler) CompleteSession(c *gin.Context) {
	asset, err := h.mediaService().Complete(c.Request.Context(), c.Param("uploadId"), c.GetInt64("userID"))
	if err != nil {
		mediaError(c, err)
		return
	}
	apierrors.RespondOK(c, service.MediaMetadata(asset))
}

func (h *UploadHandler) AbortSession(c *gin.Context) {
	if err := h.mediaService().Abort(c.Request.Context(), c.Param("uploadId"), c.GetInt64("userID")); err != nil {
		mediaError(c, err)
		return
	}
	apierrors.RespondOK(c, nil)
}

func (h *UploadHandler) Download(c *gin.Context)  { h.serveAsset(c, false) }
func (h *UploadHandler) Thumbnail(c *gin.Context) { h.serveAsset(c, true) }
func (h *UploadHandler) serveAsset(c *gin.Context, thumbnail bool) {
	asset, err := h.mediaService().AuthorizedAsset(c.Request.Context(), c.Param("fileId"), c.GetInt64("userID"))
	if err != nil {
		mediaError(c, err)
		return
	}
	objectPath, contentType, name := asset.ObjectPath, asset.MIME, asset.Name
	if thumbnail {
		if asset.ThumbnailPath == "" {
			c.JSON(404, gin.H{"code": 40400, "message": "缩略图不存在"})
			return
		}
		objectPath = asset.ThumbnailPath
		contentType = "image/jpeg"
		name = "thumbnail.jpg"
	}
	reader, err := h.minioClient.Open(c.Request.Context(), objectPath)
	if err != nil {
		mediaError(c, err)
		return
	}
	defer reader.Close()
	c.Header("Content-Type", contentType)
	c.Header("X-Content-Type-Options", "nosniff")
	if !thumbnail {
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	}
	c.Header("Cache-Control", "private, no-store")
	http.ServeContent(c.Writer, c.Request, name, asset.CreatedAt, reader)
}

func (h *UploadHandler) GroupFiles(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("groupId"), 10, 64)
	if err != nil || groupID <= 0 {
		apierrors.RespondBadRequest(c, "无效群组ID")
		return
	}
	limit, offset, err := parsePage(c)
	if err != nil {
		apierrors.RespondBadRequest(c, err.Error())
		return
	}
	db := mysql.Client.WithContext(c.Request.Context())
	var count int64
	if err = db.Model(&model.GroupMember{}).Where("group_id=? AND user_id=?", groupID, c.GetInt64("userID")).Count(&count).Error; err != nil {
		mediaError(c, err)
		return
	}
	if count == 0 {
		mediaError(c, service.ErrMediaForbidden)
		return
	}
	var assets []model.MediaAsset
	if err = db.Where("(group_id=? OR id IN (SELECT ma.asset_id FROM message_asset ma JOIN message m ON m.id=ma.message_id WHERE m.group_id=? AND m.is_revoked=0)) AND (expires_at IS NULL OR expires_at>?)", groupID, groupID, time.Now()).Order("created_at DESC,id DESC").Limit(limit).Offset(int(offset)).Find(&assets).Error; err != nil {
		mediaError(c, err)
		return
	}
	result := make([]any, 0, len(assets))
	for _, asset := range assets {
		result = append(result, service.MediaMetadata(&asset))
	}
	apierrors.RespondOK(c, result)
}
