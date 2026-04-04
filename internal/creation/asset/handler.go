// Package asset 提供游戏素材上传接口。
//
// 职责：
//   - 接受前端上传的图片/音频文件
//   - 校验 MIME 类型与文件大小（最大 10MB）
//   - 写入 ./uploads/<slug>/ 目录
//   - 返回可供前端直接引用的 URL
package asset

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// maxUploadSize 单文件最大 10 MB
const maxUploadSize = 10 << 20

// allowedMIMEs 白名单 MIME 类型
var allowedMIMEs = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
	"audio/mpeg": ".mp3",
	"audio/ogg":  ".ogg",
	"audio/wav":  ".wav",
}

// Config 素材上传配置
type Config struct {
	// UploadDir 素材存储根目录（默认 ./uploads）
	UploadDir string
	// BaseURL 对外访问 URL 前缀（默认 /uploads）
	BaseURL string
}

func (cfg *Config) defaults() {
	if cfg.UploadDir == "" {
		cfg.UploadDir = "./uploads"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "/uploads"
	}
}

// RegisterAssetRoutes 注册素材上传路由到已有的路由组。
//
//	POST /assets/:slug/upload
func RegisterAssetRoutes(rg *gin.RouterGroup, cfg Config) {
	cfg.defaults()

	rg.POST("/assets/:slug/upload", func(c *gin.Context) {
		slug := c.Param("slug")
		if slug == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "slug required"})
			return
		}

		// 限制请求体大小
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize)

		file, header, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
			return
		}
		defer file.Close()

		// 检测实际 MIME（读取前 512 字节）
		buf := make([]byte, 512)
		n, _ := file.Read(buf)
		mimeType := http.DetectContentType(buf[:n])
		// 还原文件指针
		type seeker interface{ Seek(int64, int) (int64, error) }
		if sk, ok := file.(seeker); ok {
			_, _ = sk.Seek(0, io.SeekStart)
		}

		ext, ok := allowedMIMEs[mimeType]
		if !ok {
			// 回退：用原始文件名后缀二次判断（有些浏览器 detect 不准）
			origExt := strings.ToLower(filepath.Ext(header.Filename))
			for _, v := range allowedMIMEs {
				if v == origExt {
					ext = origExt
					ok = true
					break
				}
			}
		}
		if !ok {
			c.JSON(http.StatusUnsupportedMediaType, gin.H{
				"error": fmt.Sprintf("unsupported file type: %s", mimeType),
			})
			return
		}

		// 创建目标目录
		destDir := filepath.Join(cfg.UploadDir, slug)
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create upload dir"})
			return
		}

		// 唯一文件名：时间戳 + 原始后缀
		filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
		destPath := filepath.Join(destDir, filename)

		dst, err := os.Create(destPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create file"})
			return
		}
		defer dst.Close()

		written, err := io.Copy(dst, file)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write file"})
			return
		}

		url := strings.TrimRight(cfg.BaseURL, "/") + "/" + slug + "/" + filename
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{
				"url":      url,
				"filename": filename,
				"size":     written,
				"mime":     mimeType,
			},
		})
	})
}
