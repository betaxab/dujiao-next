package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 统一响应结构
type Response struct {
	StatusCode int         `json:"status_code"` // 业务状态码
	Msg        string      `json:"msg"`         // 提示消息
	Data       interface{} `json:"data"`        // 数据内容
}

// PageResponse 分页响应结构
type PageResponse struct {
	StatusCode int         `json:"status_code"`
	Msg        string      `json:"msg"`
	Data       interface{} `json:"data"`
	Pagination Pagination  `json:"pagination"`
}

// Pagination 分页信息
type Pagination struct {
	Page      int   `json:"page"`
	PageSize  int   `json:"page_size"`
	Total     int64 `json:"total"`
	TotalPage int64 `json:"total_page"`
}

// Success 成功响应
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		StatusCode: 0,
		Msg:        "success",
		Data:       data,
	})
}

// SuccessWithMsg 成功响应（自定义消息）
func SuccessWithMsg(c *gin.Context, msg string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		StatusCode: 0,
		Msg:        msg,
		Data:       data,
	})
}

// SuccessWithPage 分页成功响应
func SuccessWithPage(c *gin.Context, data interface{}, pagination Pagination) {
	c.JSON(http.StatusOK, PageResponse{
		StatusCode: 0,
		Msg:        "success",
		Data:       data,
		Pagination: pagination,
	})
}

// Error 错误响应
func Error(c *gin.Context, statusCode int, msg string) {
	c.JSON(http.StatusOK, Response{
		StatusCode: statusCode,
		Msg:        msg,
		Data:       attachRequestID(c, nil),
	})
}

// ErrorWithData 错误响应（带数据）
func ErrorWithData(c *gin.Context, statusCode int, msg string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		StatusCode: statusCode,
		Msg:        msg,
		Data:       attachRequestID(c, data),
	})
}

// NotFound 404响应
func NotFound(c *gin.Context, msg string) {
	Error(c, CodeNotFound, msg)
}

// Unauthorized 401响应
func Unauthorized(c *gin.Context, msg string) {
	Error(c, CodeUnauthorized, msg)
}

// Forbidden 403响应
func Forbidden(c *gin.Context, msg string) {
	Error(c, CodeForbidden, msg)
}

// BadRequest 400响应
func BadRequest(c *gin.Context, msg string) {
	Error(c, CodeBadRequest, msg)
}

func attachRequestID(c *gin.Context, data interface{}) interface{} {
	requestID := ""
	if c != nil {
		if value, ok := c.Get("request_id"); ok {
			if id, ok := value.(string); ok {
				requestID = id
			}
		}
	}
	if requestID == "" {
		return data
	}
	if data == nil {
		return gin.H{"request_id": requestID}
	}
	switch v := data.(type) {
	case gin.H:
		if _, ok := v["request_id"]; !ok {
			v["request_id"] = requestID
		}
		return v
	case map[string]interface{}:
		if _, ok := v["request_id"]; !ok {
			v["request_id"] = requestID
		}
		return v
	default:
		return gin.H{
			"request_id": requestID,
			"data":       data,
		}
	}
}
