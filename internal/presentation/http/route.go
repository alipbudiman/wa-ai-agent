package route

import (
	"net/http"

	"github.com/alipbudiman/wa-ai-agent.git/internal/config"
	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"github.com/alipbudiman/wa-ai-agent.git/internal/presentation/handler"
	"github.com/gin-gonic/gin"
)

// meHandler godoc
// @Summary Health check endpoint
// @Description Return simple status for API availability
// @Tags system
// @Produce json
// @Success 200 {object} map[string]string
// @Router /v1/me [get]
func meHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// contactInfoHandler godoc
// @Summary Get WhatsApp contact info
// @Description Resolve contact details from a WhatsApp id or JID
// @Tags contact
// @Produce json
// @Param number path string true "WhatsApp Number"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/contact/info/{number} [get]
func contactInfoHandler(waClient ports.WhatsAppClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		number := c.Param("number")

		contact_info, err := handler.GetContactInfo(waClient, number)
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to get contact info")
			return
		}

		if !contact_info.Found {
			ErrorHTTPResp(c, http.StatusNotFound, "contact not found")
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "", contact_info)
	}
}

func sendMessageHandler(waClient ports.WhatsAppClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SendMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		err := handler.SendMessage(waClient, req.Number, req.Message)
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to send message")
			return
		}
		SuccessHTTPResp(c, http.StatusOK, "message sent successfully", nil)
	}
}

func RouterV1(waClient ports.WhatsAppClient, cfg *config.Config, router *gin.Engine) *gin.Engine {
	v1 := router.Group("/v1")
	{
		v1.GET("/me", meHandler)
		v1.GET("/contact/info/:number", contactInfoHandler(waClient))
		v1.POST("/message/send", sendMessageHandler(waClient))
	}

	return router
}
