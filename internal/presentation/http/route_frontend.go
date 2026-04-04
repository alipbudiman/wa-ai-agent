package route

import (
	"fmt"
	"net/http"

	"github.com/alipbudiman/wa-ai-agent.git/internal/config"
	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"github.com/alipbudiman/wa-ai-agent.git/internal/presentation/handler"
	"github.com/gin-gonic/gin"
)

func FrontendRouter(manager ports.Manager, chatCompleter ports.ChatCompleter, cfg *config.Config, router *gin.Engine) *gin.Engine {
	router.LoadHTMLGlob("internal/presentation/templates/*.gohtml")
	router.Static("/assets", "./assets")

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.gohtml", gin.H{
			"MongoDBURL": cfg.Databases.MongoDB.URI,
		})
	})

	router.GET("/access", func(c *gin.Context) {
		access_list, err := handler.GetAccessList(manager)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.HTML(http.StatusOK, "access.gohtml", gin.H{
			"AccessList":    access_list,
			"AccessBaseURL": fmt.Sprintf("%s%s/", API_BASE_PATH, ACCESS_PATH),
		})
	})

	router.GET("/artifacts", func(c *gin.Context) {
		c.HTML(http.StatusOK, "artifacts.gohtml", gin.H{
			"ProcessDiagramTitle": "Agent Artifacts",
			"ProcessDiagramPath":  "/assets/artifacts/chat_agent_process_table.png",
		})
	})

	router.GET("/reports", func(c *gin.Context) {
		runs, err := handler.GetLangSmithRuns(c.Request.Context(), chatCompleter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.HTML(http.StatusOK, "reports.gohtml", gin.H{
			"LangSmithRuns":  runs,
			"ReportsBaseURL": fmt.Sprintf("%s%s/", API_BASE_PATH, REPORTS_PATH),
		})
	})

	return router
}
