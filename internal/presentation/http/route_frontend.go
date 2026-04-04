package route

import (
	"fmt"
	"net/http"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"github.com/alipbudiman/wa-ai-agent.git/internal/presentation/handler"
	"github.com/gin-gonic/gin"
)

func FrontendRouter(manager ports.Manager, router *gin.Engine) *gin.Engine {
	router.LoadHTMLGlob("internal/presentation/templates/*.gohtml")

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.gohtml", nil)
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

	return router
}
