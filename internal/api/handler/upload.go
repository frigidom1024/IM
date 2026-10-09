package handler

import (
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"gochat/internal/pkg/errors"
	"gochat/internal/pkg/logger"
	"gochat/internal/pkg/minio"
	"gochat/internal/pkg/snowflake"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	pathImages = "images"
	pathFiles  = "files"
	pathVoices = "voices"
	pathVideos = "videos"
)

const (
	maxImageSize = 50 << 20  // 50MB
	maxFileSize  = 500 << 20 // 500MB
	maxVoiceSize = 50 << 20  // 50MB
	maxVideoSize = 1024 << 20 // 1GB
)

var allowedMIME = map[string][]string{
	"image": {"image/jpeg", "image/png", "image/gif", "image/webp"},
	"file":  {"application/pdf", "application/zip", "application/gzip", "text/plain", "application/msword", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	"voice": {"audio/mp3", "audio/wav", "audio/ogg", "audio/aac"},
	"video": {"video/mp4", "video/webm", "video/quicktime"},
}

var maxSizeMap = map[string]int64{
	"image": maxImageSize,
	"file":  maxFileSize,
	"voice": maxVoiceSize,
	"video": maxVideoSize,
}

// UploadHandler 文件上传处理器
type UploadHandler struct {
	minioClient *minio.Client
}

// NewUploadHandler 创建上传处理器
func NewUploadHandler(minioClient *minio.Client) *UploadHandler {
	return &UploadHandler{minioClient: minioClient}
}

// Upload 上传文件
func (h *UploadHandler) Upload(c *gin.Context) {
	msgType := c.PostForm("type")
	if msgType == "" {
		errors.RespondError(c, errors.New(errors.CodeUploadMissType, "缺少 type 参数"))
		return
	}

	mimes, ok := allowedMIME[msgType]
	if !ok {
		errors.RespondError(c, errors.New(errors.CodeUploadBadType, fmt.Sprintf("不支持的消息类型: %s", msgType)))
		return
	}

	maxSize, ok := maxSizeMap[msgType]
	if !ok {
		errors.RespondError(c, errors.New(errors.CodeUploadBadType, "无效的消息类型"))
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		errors.RespondBadRequest(c, "读取文件失败")
		return
	}
	defer file.Close()

	if header.Size > maxSize {
		errors.RespondError(c, errors.New(errors.CodeUploadTooLarge,
			fmt.Sprintf("文件过大，%s 类型最大允许 %dMB", msgType, maxSize>>20)))
		return
	}

	contentType := header.Header.Get("Content-Type")
	valid := false
	for _, m := range mimes {
		if contentType == m {
			valid = true
			break
		}
	}
	if !valid {
		errors.RespondError(c, errors.New(errors.CodeUploadBadMIME,
			fmt.Sprintf("不支持的文件类型: %s", contentType)))
		return
	}

	ext := path.Ext(header.Filename)
	objectPath := fmt.Sprintf("%s/%s/%d%s",
		msgType+"s",
		time.Now().Format("2006/01"),
		snowflake.MustNextID(),
		ext,
	)

	data, err := io.ReadAll(file)
	if err != nil {
		logger.Error("读取文件数据失败", zap.Error(err))
		errors.RespondError(c, errors.New(errors.CodeUploadReadFail, "读取文件失败"))
		return
	}

	meta, err := h.minioClient.Upload(c.Request.Context(), objectPath, strings.NewReader(string(data)), int64(len(data)), contentType)
	if err != nil {
		logger.Error("上传文件到 MinIO 失败", zap.String("path", objectPath), zap.Error(err))
		errors.RespondError(c, errors.New(errors.CodeUploadFail, "上传文件失败"))
		return
	}

	errors.RespondOK(c, meta)
}
