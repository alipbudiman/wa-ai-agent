package route

import "github.com/gin-gonic/gin"

type SuccessResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Request succeeded"`
	Data    any    `json:"data,omitempty"`
}

// route.go

type ErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Message string `json:"message" example:"Request failed"`
}

type SendMessageRequest struct {
	Number  string `json:"number" binding:"required"`
	Message string `json:"message" binding:"required"`
	Store   bool   `json:"store,omitempty"`
}

type SendFileRequest struct {
	Number   string `json:"number" binding:"required"`
	FilePath string `json:"file_path" binding:"required"`
	Caption  string `json:"caption,omitempty"`
}

type BroadcastMessageRequest struct {
	Message string `json:"message" binding:"required"`
}

type StoreAIMessageRequest struct {
	Message string `json:"message" binding:"required"`
	Number  string `json:"number" binding:"required"`
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

// route_manager.go

type AddAccessRequest struct {
	Name      string `json:"name" binding:"required"`
	Number    string `json:"number" binding:"required"`
	ExpiredAt int64  `json:"expired_at" binding:"required"`
}

type DeleteAccessRequest struct {
	Number string `json:"number" binding:"required"`
}

type SetInboxAddressRequest struct {
	Number string `json:"number" binding:"required"`
}

type DeleteMessageRequest struct {
	MsgID string `json:"msg_id" binding:"required"`
}

type DeleteInboxMessageRequest struct {
	JID string `json:"jid" binding:"required"`
}

type ToolDataCodeRequest struct {
	Package []string `json:"package"`
	Code    string   `json:"code"`
}

type ToolBuilderRequest struct {
	Name         string               `json:"name" binding:"required"`
	Description  string               `json:"description" binding:"required"`
	AgentType    string               `json:"agent_type" binding:"required"`
	Information  string               `json:"information,omitempty"`
	Code         *ToolDataCodeRequest `json:"code"`
	OriginalName string               `json:"original_name,omitempty"`
}

type DeleteToolBuilderRequest struct {
	AgentType string `json:"agent_type" binding:"required"`
	Name      string `json:"name" binding:"required"`
}
