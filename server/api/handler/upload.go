package handler

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"gochat/pkg/logger"
	"gochat/pkg/minio"
	"gochat/pkg/snowflake"

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
	maxImageSize = 10 << 20  // 10MB
	maxFileSize  = 100 << 20 // 100MB
	maxVoiceSize = 10 << 20  // 10MB
	maxVideoSize = 200 << 20 // 200MB
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
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "缺少 type 参数"})
		return
	}

	mimes, ok := allowedMIME[msgType]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": fmt.Sprintf("不支持的消息类型: %s", msgType)})
		return
	}

	maxSize, ok := maxSizeMap[msgType]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无效的消息类型"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "读取文件失败"})
		return
	}
	defer file.Close()

	if header.Size > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": fmt.Sprintf("文件过大，%s 类型最大允许 %dMB", msgType, maxSize>>20),
		})
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
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": fmt.Sprintf("不支持的文件类型: %s", contentType),
		})
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
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "读取文件失败"})
		return
	}

	meta, err := h.minioClient.Upload(c.Request.Context(), objectPath, strings.NewReader(string(data)), int64(len(data)), contentType)
	if err != nil {
		logger.Error("上传文件到 MinIO 失败", zap.String("path", objectPath), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "上传文件失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": meta})
}
