package route

import (
	"fmt"
	"net/http"
	"net/url"
	"runtime"

	"github.com/alipbudiman/wa-ai-agent.git/internal/config"
	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"github.com/alipbudiman/wa-ai-agent.git/internal/presentation/handler"
	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
)

// meHandler godoc
// @Summary Get self information
// @Description Return WhatsApp account information of the connected bot
// @Tags system
// @Produce json
// @Success 200 {object} SuccessResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/me [get]
func meHandler(waClient ports.WhatsAppClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		self_information, err := handler.GetSelfInformation(waClient)
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to get self information")
			return
		}
		os_profile := proto.String(fmt.Sprintf("Chrome (%v)", runtime.GOOS))
		contact_info, err := handler.GetContactInfo(waClient, self_information.String())
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to get contact info")
			return
		}
		SuccessHTTPResp(c, http.StatusOK, "", gin.H{
			"bot_jid":          self_information,
			"os_profile":       os_profile,
			"bot_contact_info": contact_info,
		})
	}
}

// contactInfoHandler godoc
// @Summary Get WhatsApp contact info
// @Description Resolve contact details from a WhatsApp number or full JID (e.g. 6281234567890, 867051314767696@lid)
// @Tags contact
// @Produce json
// @Param number path string true "WhatsApp Number or JID"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/contact/info/{number} [get]
func contactInfoHandler(waClient ports.WhatsAppClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		number, err := url.PathUnescape(c.Param("number"))
		if err != nil || number == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid number or jid parameter")
			return
		}

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

// sendMessageHandler godoc
// @Summary Send a WhatsApp message
// @Description Send a text message to a specific WhatsApp number
// @Tags message
// @Accept json
// @Produce json
// @Param request body SendMessageRequest true "Send message payload"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/message/send [post]
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

// broadcastMessageHandler godoc
// @Summary Broadcast a WhatsApp message
// @Description Send a text message to all saved contacts
// @Tags message
// @Accept json
// @Produce json
// @Param request body BroadcastMessageRequest true "Broadcast message payload"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/message/broadcast [post]
func broadcastMessageHandler(waClient ports.WhatsAppClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req BroadcastMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		err := handler.BroadcastMessage(waClient, req.Message)
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
		v1.GET("/me", meHandler(waClient))
		v1.GET("/contact/info/:number", contactInfoHandler(waClient))
		v1.POST("/message/send", sendMessageHandler(waClient))
		v1.POST("/message/broadcast", broadcastMessageHandler(waClient))
	}

	return router
}
