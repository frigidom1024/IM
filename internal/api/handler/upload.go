package handler

import (
	"github.com/gin-gonic/gin"
	"gochat/internal/logic/model"
	"gochat/internal/logic/service"
	apierrors "gochat/internal/pkg/errors"
	"gochat/internal/pkg/minio"
	"gochat/internal/pkg/mysql"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	maxImageSize = 50 << 20
	maxFileSize  = 500 << 20
	maxVoiceSize = 50 << 20
	maxVideoSize = 1024 << 20
)

var allowedMIME = map[string][]string{
	"image": {"image/jpeg", "image/png", "image/gif", "image/webp"},
	"file":  {"application/pdf", "application/zip", "application/gzip", "text/plain", "application/msword", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	"voice": {"audio/mp3", "audio/mpeg", "audio/wav", "audio/x-wav", "audio/ogg", "audio/aac"},
	"video": {"video/mp4", "video/webm", "video/quicktime"},
}
var maxSizeMap = map[string]int64{"image": maxImageSize, "file": maxFileSize, "voice": maxVoiceSize, "video": maxVideoSize}

type UploadHandler struct{ minioClient *minio.Client }

func NewUploadHandler(client *minio.Client) *UploadHandler {
	return &UploadHandler{minioClient: client}
}

// Upload preserves the multipart API while streaming through a disk-backed
// session. The response now contains an authenticated download route.
func (h *UploadHandler) Upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxVideoSize+(1<<20))
	if err := c.Request.ParseMultipartForm(8 << 20); err != nil {
		apierrors.RespondBadRequest(c, "无效或过大的上传请求")
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	kind := c.PostForm("type")
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		apierrors.RespondBadRequest(c, "缺少文件")
		return
	}
	defer file.Close()
	contentType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil || !validMedia(kind, contentType, header.Size) {
		apierrors.RespondBadRequest(c, "无效的文件类型或大小")
		return
	}
	groupID := int64(0)
	if value := c.PostForm("group_id"); value != "" {
		groupID, err = strconv.ParseInt(value, 10, 64)
		if err != nil || groupID <= 0 {
			apierrors.RespondBadRequest(c, "无效群组ID")
			return
		}
	}
	name := filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
	if len(name) > 255 {
		apierrors.RespondBadRequest(c, "文件名过长")
		return
	}
	session := &model.UploadSession{UserID: c.GetInt64("userID"), GroupID: groupID, Name: name, Type: kind, MIME: contentType, Size: header.Size}
	svc := h.mediaService()
	if err = svc.CreateSession(c.Request.Context(), session); err != nil {
		mediaError(c, err)
		return
	}
	completed := false
	defer func() {
		if !completed {
			svc.Abort(c.Request.Context(), session.ID, session.UserID)
		}
	}()
	dest, err := os.OpenFile(session.TempPath, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		mediaError(c, err)
		return
	}
	written, copyErr := io.Copy(dest, io.LimitReader(file, session.Size+1))
	syncErr := dest.Sync()
	closeErr := dest.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || written != session.Size {
		apierrors.RespondBadRequest(c, "上传不完整")
		return
	}
	if err = mysql.Client.WithContext(c.Request.Context()).Model(session).Update("offset", written).Error; err != nil {
		mediaError(c, err)
		return
	}
	asset, err := svc.Complete(c.Request.Context(), session.ID, session.UserID)
	if err != nil {
		mediaError(c, err)
		return
	}
	completed = true
	apierrors.RespondOK(c, service.MediaMetadata(asset))
}
