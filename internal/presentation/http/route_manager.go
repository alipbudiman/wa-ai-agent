package route

import (
	"net/http"

	"github.com/alipbudiman/wa-ai-agent.git/internal/config"
	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"github.com/alipbudiman/wa-ai-agent.git/internal/presentation/handler"
	"github.com/gin-gonic/gin"
)

const (
	API_BASE_PATH  string = "/api"
	ACCESS_PATH    string = "/access"
	REPORTS_PATH   string = "/reports"
	BOT_SETUP_PATH string = "/bot-setup"
)

var (
	// predefined routes for BOT_SETUP_PATH
	setupRoute = SetupRoute{
		Origin: BOT_SETUP_PATH,
		Inbox:  "/inbox",
	}
)

type SetupRoute struct {
	Origin string `json:"origin"`
	Inbox  string `json:"inbox"`
}

func getAccessHandler(manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		AccessList, err := manager.GetAccessList()
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to get access list")
			return
		}
		SuccessHTTPResp(c, http.StatusOK, "access list retrieved successfully", AccessList)
	}
}

func addAccessHandler(manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req AddAccessRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		if err := handler.AddAccess(manager, req.Name, req.Number, req.ExpiredAt); err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to add access")
			return
		}

		SuccessHTTPResp(c, http.StatusCreated, "access added successfully", nil)
	}
}

func deleteAccessHandler(manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req DeleteAccessRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		if err := handler.DeleteAccess(manager, req.Number); err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to delete access")
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "access deleted successfully", nil)
	}
}

func getLangSmithRunsHandler(chatCompleter ports.ChatCompleter) gin.HandlerFunc {
	return func(c *gin.Context) {
		runs, err := handler.GetLangSmithRuns(c.Request.Context(), chatCompleter)
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to get LangSmith runs")
			return
		}
		SuccessHTTPResp(c, http.StatusOK, "LangSmith runs retrieved successfully", runs)
	}
}

func getLangSmithSelectRunHandler(chatCompleter ports.ChatCompleter) gin.HandlerFunc {
	return func(c *gin.Context) {
		runID := c.Param("runID")
		if runID == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "runID is required")
			return
		}

		run, err := handler.GetLangSmithSelectRun(c.Request.Context(), chatCompleter, runID)
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to get LangSmith run detail")
			return
		}
		SuccessHTTPResp(c, http.StatusOK, "LangSmith run detail retrieved successfully", run)
	}
}

func getInboxAddressHandler(manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		jid, err := handler.GetInboxAddress(manager)
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to get inbox address")
			return
		}
		SuccessHTTPResp(c, http.StatusOK, "inbox address retrieved successfully", jid.String())
	}
}

func setInboxAddressHandler(manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SetInboxAddressRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		if err := handler.SetInboxAddress(manager, req.Number); err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to set inbox address")
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "inbox address set successfully", nil)
	}
}

func deleteInboxAddressHandler(manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := handler.ClearInboxAddress(manager); err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to delete inbox address")
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "inbox address deleted successfully", nil)
	}
}

func getInboxMessagesHandler(manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		messages, err := handler.GetInboxMessages(manager)
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to get inbox messages")
			return
		}
		SuccessHTTPResp(c, http.StatusOK, "inbox messages retrieved successfully", messages)
	}
}

func deleteMessage(waClient ports.WhatsAppClient, manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req DeleteMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		if err := handler.DeleteMessage(manager, req.MsgID); err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to delete inbox message")
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "inbox message deleted successfully", nil)
	}
}

func deleteAllInboxMessagesByJIDHandler(waClient ports.WhatsAppClient, manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req DeleteInboxMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		jid, err := waClient.ParseJID(req.JID)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid jid format")
			return
		}

		if err := handler.DeleteAllInboxMessagesByJID(manager, jid); err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to delete inbox messages by JID")
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "inbox messages deleted successfully", nil)
	}
}

func deleteAllInboxMessagesHandler(manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := handler.DeleteAllInboxMessages(manager); err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to delete all inbox messages")
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "all inbox messages deleted successfully", nil)
	}
}

func getUnreadInboxMessagesHandler(waClient ports.WhatsAppClient, manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		jidParam := c.Param("jid")
		if jidParam == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "jid query parameter is required")
			return
		}

		jid, err := waClient.ParseJID(jidParam)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid jid format")
			return
		}

		unreadMsgIDs, err := handler.GetUnreadInboxMessages(manager, jid)
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to get unread inbox messages")
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "unread inbox messages retrieved successfully", unreadMsgIDs)
	}
}

func markInboxMessageAsReadHandler(manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		msgIDParam := c.Param("msg_id")
		if msgIDParam == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "msg_id query parameter is required")
			return
		}

		if err := handler.MarkInboxMessageAsRead(manager, msgIDParam); err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to mark inbox message as read")
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "inbox message marked as read successfully", nil)
	}
}

/*
ManagerRouter sets up the routes for manager-related operations under the /api path.
*/
func ManagerRouter(manager ports.Manager, chatCompleter ports.ChatCompleter, waClient ports.WhatsAppClient, cfg *config.Config, router *gin.Engine) *gin.Engine {
	r := router.Group(API_BASE_PATH)
	{
		access := r.Group(ACCESS_PATH)
		{
			access.GET("/", getAccessHandler(manager))
			access.PUT("/", addAccessHandler(manager))
			access.DELETE("/", deleteAccessHandler(manager))
		}

		reports := r.Group(REPORTS_PATH)
		{
			reports.GET("/langsmith", getLangSmithRunsHandler(chatCompleter))
			reports.GET("/langsmith/:runID", getLangSmithSelectRunHandler(chatCompleter))
		}

		botSetup := r.Group(BOT_SETUP_PATH)
		{

			inbox := botSetup.Group(setupRoute.Inbox)
			{
				inbox.GET("/", getInboxMessagesHandler(manager))
				inbox.DELETE("/", deleteMessage(waClient, manager))
				inbox.DELETE("/jid", deleteAllInboxMessagesByJIDHandler(waClient, manager))
				inbox.DELETE("/all", deleteAllInboxMessagesHandler(manager))
				inbox.GET("/unread/:jid", getUnreadInboxMessagesHandler(waClient, manager))
				inbox.GET("/read/:msg_id", markInboxMessageAsReadHandler(manager))
				inbox.GET("/address", getInboxAddressHandler(manager))
				inbox.PUT("/address", setInboxAddressHandler(manager))
				inbox.DELETE("/address", deleteInboxAddressHandler(manager))
			}
		}

	}
	return router
}
