package route

import (
	"net/http"

	"github.com/alipbudiman/wa-ai-agent.git/internal/config"
	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"github.com/alipbudiman/wa-ai-agent.git/internal/presentation/handler"
	"github.com/gin-gonic/gin"
)

const (
	API_BASE_PATH string = "/api"
	ACCESS_PATH   string = "/access"
	REPORTS_PATH  string = "/reports"
)

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

/*
ManagerRouter sets up the routes for manager-related operations under the /api path.
*/
func ManagerRouter(manager ports.Manager, chatCompleter ports.ChatCompleter, cfg *config.Config, router *gin.Engine) *gin.Engine {
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
	}
	return router
}
