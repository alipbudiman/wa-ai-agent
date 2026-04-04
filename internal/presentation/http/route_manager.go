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

/*
ManagerRouter sets up the routes for manager-related operations under the /api path.
*/
func ManagerRouter(manager ports.Manager, cfg *config.Config, router *gin.Engine) *gin.Engine {
	r := router.Group(API_BASE_PATH)
	{
		access := r.Group(ACCESS_PATH)
		{
			access.GET("/", getAccessHandler(manager))
			access.PUT("/", addAccessHandler(manager))
			access.DELETE("/", deleteAccessHandler(manager))
		}
	}
	return router
}
