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

const (
	BOT_ROUTE_BASE string = "/v1"
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
func sendMessageHandler(waClient ports.WhatsAppClient, manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SendMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}
		err := handler.SendMessage(waClient, manager, req.Number, req.Message, req.Store)
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to send message")
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "message sent successfully", nil)
	}
}

// sendFileHandler godoc
// @Summary Send a WhatsApp file
// @Description Send a file message to a specific WhatsApp number
// @Tags message
// @Accept json
// @Produce json
// @Param request body SendFileRequest true "Send file payload"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/message/send-file [post]
func sendFileHandler(waClient ports.WhatsAppClient, manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SendFileRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}
		err := handler.SendFile(waClient, manager, req.Number, req.FilePath, req.Caption)
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to send file")
			return
		}
		SuccessHTTPResp(c, http.StatusOK, "file sent successfully", nil)
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

func getAllCommandsHandler(manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		commands, err := handler.GetAllCommands(manager)
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to get commands")
			return
		}
		SuccessHTTPResp(c, http.StatusOK, "commands retrieved successfully", commands)
	}
}

func storeAIMessageHandler(manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req StoreAIMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		err := handler.StoreAIMessageToInbox(manager, req.Message, req.Number)
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to store AI message")
			return
		}
		SuccessHTTPResp(c, http.StatusOK, "AI message stored successfully", nil)
	}
}

func showAllContactsHandler(waClient ports.WhatsAppClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		contacts, err := handler.GetAllContacts(waClient)
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to get contacts")
			return
		}
		SuccessHTTPResp(c, http.StatusOK, "contacts retrieved successfully", contacts)
	}
}

func injectAIConversationHandler(waClient ports.WhatsAppClient, chatCompleter ports.ChatCompleter) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req StoreAIMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		normalizedJID, err := waClient.ParseJID(req.Number)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid number or jid")
			return
		}

		success, err := handler.InjectAIConversation(chatCompleter, req.Message, normalizedJID.ToNonAD().String())
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to inject AI conversation")
			return
		}

		if !success {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to inject AI conversation")
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "AI conversation injected successfully", nil)
	}
}

func RouterV1(waClient ports.WhatsAppClient, manager ports.Manager, chatCompleter ports.ChatCompleter, cfg *config.Config, router *gin.Engine) *gin.Engine {
	v1 := router.Group(BOT_ROUTE_BASE)
	{
		v1.GET("/me", meHandler(waClient))
		v1.GET("/contact/info/:number", contactInfoHandler(waClient))
		v1.POST("/message/send", sendMessageHandler(waClient, manager))
		v1.POST("/message/broadcast", broadcastMessageHandler(waClient))
		v1.POST("/message/send-file", sendFileHandler(waClient, manager))
		v1.GET("/commands", getAllCommandsHandler(manager))
		v1.POST("/store-ai-message", storeAIMessageHandler(manager))
		v1.GET("/show-contacts", showAllContactsHandler(waClient))
		v1.POST("/inject-conversation", injectAIConversationHandler(waClient, chatCompleter))
	}

	return router
}
