package util

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	defaultLimit = 20
	maxLimit     = 200
)

// ParsePage 从 Gin 查询参数解析分页参数，并做边界校正。
//
// 参数：
//   - limit：每页条数，默认 20，上限 200
//   - offset：偏移量，默认 0，不允许负数
//
// 用法：
//
//	limit, offset := util.ParsePage(c)
//	db.Limit(limit).Offset(offset).Find(&items)
func ParsePage(c *gin.Context) (limit, offset int) {
	limit, _ = strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ = strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > maxLimit {
		limit = defaultLimit
	}
	if offset < 0 {
		offset = 0
	}
	return
}
