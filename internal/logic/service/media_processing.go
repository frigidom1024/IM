package service

import (
	"context"
	"encoding/json"
	"fmt"
	"gochat/internal/logic/model"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"strconv"
	"time"
)

var mediaProcessingSlots = make(chan struct{}, 2)

// processMedia operates on agent-generated local paths, with bounded pixel and
// runtime limits. The original user filename never enters an executable command.
func processMedia(ctx context.Context, filename, kind string, asset *model.MediaAsset) (output, thumbnail string, err error) {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	select {
	case mediaProcessingSlots <- struct{}{}:
		defer func() { <-mediaProcessingSlots }()
	case <-bounded.Done():
		return "", "", bounded.Err()
	}
	output = filename
	if kind == "image" && asset.MIME != "image/webp" {
		file, e := os.Open(filename)
		if e != nil {
			return "", "", e
		}
		cfg, _, e := image.DecodeConfig(file)
		if e != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 20_000_000 {
			file.Close()
			return "", "", fmt.Errorf("无效或像素过大的图片")
		}
		file.Seek(0, 0)
		img, _, e := image.Decode(file)
		file.Close()
		if e != nil {
			return "", "", e
		}
		asset.Width, asset.Height = cfg.Width, cfg.Height
		width, height := cfg.Width, cfg.Height
		if width > 320 || height > 320 {
			if width >= height {
				height = height * 320 / width
				width = 320
			} else {
				width = width * 320 / height
				height = 320
			}
		}
		if width < 1 {
			width = 1
		}
		if height < 1 {
			height = 1
		}
		thumb := image.NewRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				thumb.Set(x, y, img.At(img.Bounds().Min.X+x*cfg.Width/width, img.Bounds().Min.Y+y*cfg.Height/height))
			}
		}
		thumbnail = filename + ".thumb.jpg"
		dest, e := os.Create(thumbnail)
		if e != nil {
			return "", "", e
		}
		e = jpeg.Encode(dest, thumb, &jpeg.Options{Quality: 80})
		closeErr := dest.Close()
		if e != nil {
			return "", "", e
		}
		return output, thumbnail, closeErr
	}
	if kind != "voice" && kind != "video" && kind != "image" {
		return output, "", nil
	}
	// Force a known demuxer so uploaded playlists cannot load local files.
	demuxer := map[string]string{"audio/mp3": "mp3", "audio/mpeg": "mp3", "audio/wav": "wav", "audio/x-wav": "wav", "audio/ogg": "ogg", "audio/aac": "aac", "video/mp4": "mov", "video/quicktime": "mov", "video/webm": "matroska", "image/webp": "webp_pipe"}[asset.MIME]
	if demuxer == "" {
		return "", "", fmt.Errorf("不支持的媒体格式")
	}
	data, e := exec.CommandContext(bounded, "ffprobe", "-v", "error", "-max_alloc", "268435456", "-f", demuxer, "-protocol_whitelist", "file,pipe", "-show_entries", "format=duration:stream=width,height,codec_type", "-of", "json", filename).Output()
	if e != nil {
		return "", "", fmt.Errorf("媒体探测失败，请安装 ffprobe 并上传有效媒体")
	}
	var probe struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Width, Height int
			CodecType     string `json:"codec_type"`
		} `json:"streams"`
	}
	if json.Unmarshal(data, &probe) != nil {
		return "", "", fmt.Errorf("媒体元数据无效")
	}
	asset.Duration, _ = strconv.ParseFloat(probe.Format.Duration, 64)
	if kind != "image" && (asset.Duration <= 0 || asset.Duration > 3600) {
		return "", "", fmt.Errorf("媒体时长必须在0到3600秒之间")
	}
	hasAudio := false
	for _, stream := range probe.Streams {
		if stream.CodecType == "audio" {
			hasAudio = true
		}
		if stream.CodecType == "video" {
			asset.Width, asset.Height = stream.Width, stream.Height
		}
	}
	if kind == "voice" && !hasAudio {
		return "", "", fmt.Errorf("无效音频")
	}
	if (kind == "video" || kind == "image") && (asset.Width <= 0 || asset.Height <= 0 || int64(asset.Width)*int64(asset.Height) > 20_000_000) {
		return "", "", fmt.Errorf("无效或像素过大的媒体")
	}
	if kind == "video" {
		output = filename + ".mp4"
		e = exec.CommandContext(bounded, "ffmpeg", "-v", "error", "-nostdin", "-y", "-max_alloc", "268435456", "-threads", "2", "-f", demuxer, "-protocol_whitelist", "file,pipe", "-i", filename, "-map", "0:v:0", "-map", "0:a?", "-c:v", "libx264", "-preset", "veryfast", "-threads", "2", "-r", "30", "-c:a", "aac", "-fs", "1073741824", "-movflags", "+faststart", output).Run()
		if e != nil {
			return "", "", fmt.Errorf("视频转码失败")
		}
		asset.MIME = "video/mp4"
		demuxer = "mov"
		stat, statErr := os.Stat(output)
		if statErr != nil || stat.Size() >= 1<<30 {
			return "", "", fmt.Errorf("转码结果超过1GiB")
		}
	}
	if kind == "image" || kind == "video" {
		thumbnail = filename + ".thumb.jpg"
		e = exec.CommandContext(bounded, "ffmpeg", "-v", "error", "-nostdin", "-y", "-max_alloc", "268435456", "-threads", "2", "-f", demuxer, "-protocol_whitelist", "file,pipe", "-i", output, "-frames:v", "1", "-vf", "scale=320:320:force_original_aspect_ratio=decrease", "-threads", "2", thumbnail).Run()
		if e != nil {
			return "", "", fmt.Errorf("缩略图生成失败")
		}
	}
	return output, thumbnail, nil
}
