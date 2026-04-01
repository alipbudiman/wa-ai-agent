package route

import "github.com/gin-gonic/gin"

type SuccessResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Request succeeded"`
	Data    any    `json:"data,omitempty"`
}

type ErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Message string `json:"message" example:"Request failed"`
}

type SendMessageRequest struct {
	Number  string `json:"number" binding:"required"`
	Message string `json:"message" binding:"required"`
}

func SuccessHTTPResp(c *gin.Context, statusCode int, message string, data any) {
	c.JSON(statusCode, SuccessResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func ErrorHTTPResp(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, ErrorResponse{
		Success: false,
		Message: message,
	})
}
