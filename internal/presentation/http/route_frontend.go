package route

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alipbudiman/wa-ai-agent.git/internal/config"
	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"github.com/alipbudiman/wa-ai-agent.git/internal/presentation/handler"
	eventsrunner "github.com/alipbudiman/wa-ai-agent.git/internal/presentation/run"
	"github.com/gin-gonic/gin"
)

const (
	playgroundRootDir         = "playground"
	playgroundPreviewMaxBytes = 512 * 1024
)

type playgroundEntryDTO struct {
	Name         string `json:"name"`
	RelativePath string `json:"relative_path"`
	IsDir        bool   `json:"is_dir"`
	Size         int64  `json:"size"`
	ModifiedAt   string `json:"modified_at"`
}

type playgroundDeleteRequest struct {
	Path string `json:"path" binding:"required"`
}

type playgroundCreateFolderRequest struct {
	ParentPath string `json:"parent_path"`
	FolderName string `json:"folder_name" binding:"required"`
}

type brainProcessDiagramResponse struct {
	Status  string `json:"status"`
	Mermaid string `json:"mermaid"`
}

// proxyBrainDiagram fetches a mermaid process diagram from a brain API endpoint and forwards it to the client.
func proxyBrainDiagram(c *gin.Context, endpoint string) {
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		ErrorHTTPResp(c, http.StatusBadGateway, "failed to build process diagram request")
		return
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		ErrorHTTPResp(c, http.StatusBadGateway, fmt.Sprintf("failed to fetch process diagram: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "process diagram service returned an error")
			return
		}
		bodyText := strings.TrimSpace(string(bodyBytes))
		if bodyText == "" {
			bodyText = fmt.Sprintf("upstream status %d", resp.StatusCode)
		}
		ErrorHTTPResp(c, http.StatusBadGateway, "process diagram service error: "+bodyText)
		return
	}

	var payload brainProcessDiagramResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		ErrorHTTPResp(c, http.StatusBadGateway, "invalid process diagram response payload")
		return
	}

	mermaidSource := strings.TrimSpace(payload.Mermaid)
	if mermaidSource == "" {
		ErrorHTTPResp(c, http.StatusBadGateway, "empty process diagram payload")
		return
	}

	SuccessHTTPResp(c, http.StatusOK, "", gin.H{"mermaid": mermaidSource})
}

func normalizePlaygroundPath(rawPath string) string {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return ""
	}

	unixLike := strings.ReplaceAll(trimmed, "\\", "/")
	normalized := path.Clean("/" + unixLike)
	normalized = strings.TrimPrefix(normalized, "/")
	if normalized == "." {
		return ""
	}

	return normalized
}

func resolvePlaygroundPath(rawPath string) (rootAbsPath string, targetAbsPath string, relativePath string, err error) {
	rootAbsPath, err = filepath.Abs(playgroundRootDir)
	if err != nil {
		return "", "", "", err
	}

	normalizedRel := normalizePlaygroundPath(rawPath)
	joinedPath := rootAbsPath
	if normalizedRel != "" {
		joinedPath = filepath.Join(rootAbsPath, filepath.FromSlash(normalizedRel))
	}

	targetAbsPath, err = filepath.Abs(joinedPath)
	if err != nil {
		return "", "", "", err
	}

	relativePath, err = filepath.Rel(rootAbsPath, targetAbsPath)
	if err != nil {
		return "", "", "", err
	}

	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", "", "", fmt.Errorf("invalid path")
	}

	relativePath = filepath.ToSlash(relativePath)
	if relativePath == "." {
		relativePath = ""
	}

	return rootAbsPath, targetAbsPath, relativePath, nil
}

func playgroundParentPath(currentPath string) string {
	if currentPath == "" {
		return ""
	}

	parent := filepath.ToSlash(filepath.Dir(currentPath))
	if parent == "." {
		return ""
	}

	return parent
}

func isTextPreview(contentType string, content []byte) bool {
	normalizedType := strings.ToLower(strings.TrimSpace(contentType))
	if strings.HasPrefix(normalizedType, "text/") {
		return true
	}

	if strings.Contains(normalizedType, "json") || strings.Contains(normalizedType, "xml") || strings.Contains(normalizedType, "yaml") || strings.Contains(normalizedType, "javascript") {
		return true
	}

	if len(content) == 0 {
		return true
	}

	if bytes.IndexByte(content, 0) >= 0 {
		return false
	}

	return utf8.Valid(content)
}

func normalizePlaygroundNodeName(rawName string) (string, error) {
	trimmed := strings.TrimSpace(rawName)
	if trimmed == "" {
		return "", fmt.Errorf("name is required")
	}

	if trimmed == "." || trimmed == ".." {
		return "", fmt.Errorf("invalid name")
	}

	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") || strings.ContainsRune(trimmed, 0) {
		return "", fmt.Errorf("invalid name")
	}

	return trimmed, nil
}

func FrontendRouter(manager ports.Manager, chatCompleter ports.ChatCompleter, cfg *config.Config, router *gin.Engine) *gin.Engine {
	router.LoadHTMLGlob("internal/presentation/templates/*.gohtml")
	router.Static("/assets", "./assets")

	router.GET("/artifacts/playground/list", func(c *gin.Context) {
		_, targetAbsPath, currentPath, err := resolvePlaygroundPath(c.Query("path"))
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid playground path")
			return
		}

		folderInfo, err := os.Stat(targetAbsPath)
		if err != nil {
			if os.IsNotExist(err) {
				ErrorHTTPResp(c, http.StatusNotFound, "playground path not found")
				return
			}
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to inspect playground path")
			return
		}

		if !folderInfo.IsDir() {
			ErrorHTTPResp(c, http.StatusBadRequest, "target path is not a folder")
			return
		}

		dirEntries, err := os.ReadDir(targetAbsPath)
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to read playground folder")
			return
		}

		entries := make([]playgroundEntryDTO, 0, len(dirEntries))
		for _, dirEntry := range dirEntries {
			entryInfo, infoErr := dirEntry.Info()
			if infoErr != nil {
				continue
			}

			relativeEntryPath := dirEntry.Name()
			if currentPath != "" {
				relativeEntryPath = currentPath + "/" + dirEntry.Name()
			}

			entries = append(entries, playgroundEntryDTO{
				Name:         dirEntry.Name(),
				RelativePath: filepath.ToSlash(relativeEntryPath),
				IsDir:        dirEntry.IsDir(),
				Size:         entryInfo.Size(),
				ModifiedAt:   entryInfo.ModTime().UTC().Format(time.RFC3339),
			})
		}

		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir != entries[j].IsDir {
				return entries[i].IsDir
			}

			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		})

		SuccessHTTPResp(c, http.StatusOK, "", gin.H{
			"current_path": currentPath,
			"parent_path":  playgroundParentPath(currentPath),
			"entries":      entries,
		})
	})

	router.GET("/artifacts/playground/file", func(c *gin.Context) {
		queryPath := strings.TrimSpace(c.Query("path"))
		if queryPath == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "file path is required")
			return
		}

		_, targetAbsPath, relativePath, err := resolvePlaygroundPath(queryPath)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid playground file path")
			return
		}

		fileInfo, err := os.Stat(targetAbsPath)
		if err != nil {
			if os.IsNotExist(err) {
				ErrorHTTPResp(c, http.StatusNotFound, "file not found")
				return
			}
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to inspect file")
			return
		}

		if fileInfo.IsDir() {
			ErrorHTTPResp(c, http.StatusBadRequest, "target path is a folder")
			return
		}

		file, err := os.Open(targetAbsPath)
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to open file")
			return
		}
		defer file.Close()

		previewBytes, err := io.ReadAll(io.LimitReader(file, playgroundPreviewMaxBytes+1))
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to read file")
			return
		}

		isTruncated := false
		if int64(len(previewBytes)) > playgroundPreviewMaxBytes {
			previewBytes = previewBytes[:playgroundPreviewMaxBytes]
			isTruncated = true
		}

		contentType := http.DetectContentType(previewBytes)
		textPreviewable := isTextPreview(contentType, previewBytes)

		previewContent := ""
		if textPreviewable {
			previewContent = string(previewBytes)
		}

		SuccessHTTPResp(c, http.StatusOK, "", gin.H{
			"name":          fileInfo.Name(),
			"relative_path": relativePath,
			"size":          fileInfo.Size(),
			"modified_at":   fileInfo.ModTime().UTC().Format(time.RFC3339),
			"content_type":  contentType,
			"is_text":       textPreviewable,
			"is_truncated":  isTruncated,
			"content":       previewContent,
		})
	})

	router.GET("/artifacts/playground/open", func(c *gin.Context) {
		queryPath := strings.TrimSpace(c.Query("path"))
		if queryPath == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "file path is required")
			return
		}

		_, targetAbsPath, _, err := resolvePlaygroundPath(queryPath)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid playground file path")
			return
		}

		fileInfo, err := os.Stat(targetAbsPath)
		if err != nil {
			if os.IsNotExist(err) {
				ErrorHTTPResp(c, http.StatusNotFound, "file not found")
				return
			}
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to inspect file")
			return
		}

		if fileInfo.IsDir() {
			ErrorHTTPResp(c, http.StatusBadRequest, "target path is a folder")
			return
		}

		c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", fileInfo.Name()))
		c.File(targetAbsPath)
	})

	router.DELETE("/artifacts/playground/delete", func(c *gin.Context) {
		var req playgroundDeleteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		_, targetAbsPath, relativePath, err := resolvePlaygroundPath(req.Path)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid playground path")
			return
		}

		if relativePath == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "cannot delete playground root folder")
			return
		}

		targetInfo, err := os.Stat(targetAbsPath)
		if err != nil {
			if os.IsNotExist(err) {
				ErrorHTTPResp(c, http.StatusNotFound, "target not found")
				return
			}
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to inspect target")
			return
		}

		if targetInfo.IsDir() {
			err = os.RemoveAll(targetAbsPath)
		} else {
			err = os.Remove(targetAbsPath)
		}

		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to delete target")
			return
		}

		deletedType := "file"
		if targetInfo.IsDir() {
			deletedType = "folder"
		}

		SuccessHTTPResp(c, http.StatusOK, "deleted successfully", gin.H{
			"deleted_type":  deletedType,
			"relative_path": relativePath,
		})
	})

	router.POST("/artifacts/playground/folder", func(c *gin.Context) {
		var req playgroundCreateFolderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		_, parentAbsPath, parentRelativePath, err := resolvePlaygroundPath(req.ParentPath)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid parent path")
			return
		}

		parentInfo, err := os.Stat(parentAbsPath)
		if err != nil {
			if os.IsNotExist(err) {
				ErrorHTTPResp(c, http.StatusNotFound, "parent path not found")
				return
			}
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to inspect parent path")
			return
		}

		if !parentInfo.IsDir() {
			ErrorHTTPResp(c, http.StatusBadRequest, "parent path must be a folder")
			return
		}

		folderName, err := normalizePlaygroundNodeName(req.FolderName)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid folder name")
			return
		}

		relativeFolderPath := folderName
		if parentRelativePath != "" {
			relativeFolderPath = parentRelativePath + "/" + folderName
		}

		_, targetFolderAbsPath, targetRelativePath, err := resolvePlaygroundPath(relativeFolderPath)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid folder target")
			return
		}

		if _, statErr := os.Stat(targetFolderAbsPath); statErr == nil {
			ErrorHTTPResp(c, http.StatusConflict, "folder already exists")
			return
		} else if !os.IsNotExist(statErr) {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to inspect folder target")
			return
		}

		if err := os.Mkdir(targetFolderAbsPath, 0o755); err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to create folder")
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "folder created successfully", gin.H{
			"parent_path":   parentRelativePath,
			"relative_path": targetRelativePath,
			"name":          folderName,
		})
	})

	router.POST("/artifacts/playground/upload", func(c *gin.Context) {
		_, folderAbsPath, folderRelativePath, err := resolvePlaygroundPath(c.PostForm("path"))
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid target folder path")
			return
		}

		folderInfo, err := os.Stat(folderAbsPath)
		if err != nil {
			if os.IsNotExist(err) {
				ErrorHTTPResp(c, http.StatusNotFound, "target folder not found")
				return
			}
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to inspect target folder")
			return
		}

		if !folderInfo.IsDir() {
			ErrorHTTPResp(c, http.StatusBadRequest, "target path must be a folder")
			return
		}

		fileHeader, err := c.FormFile("file")
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "file is required")
			return
		}

		sanitizedFileName, err := normalizePlaygroundNodeName(path.Base(strings.ReplaceAll(fileHeader.Filename, "\\", "/")))
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid file name")
			return
		}

		relativeFilePath := sanitizedFileName
		if folderRelativePath != "" {
			relativeFilePath = folderRelativePath + "/" + sanitizedFileName
		}

		_, targetFileAbsPath, targetRelativePath, err := resolvePlaygroundPath(relativeFilePath)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid upload target")
			return
		}

		if _, statErr := os.Stat(targetFileAbsPath); statErr == nil {
			ErrorHTTPResp(c, http.StatusConflict, "file already exists")
			return
		} else if !os.IsNotExist(statErr) {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to inspect upload target")
			return
		}

		if err := c.SaveUploadedFile(fileHeader, targetFileAbsPath); err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to save uploaded file")
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "file uploaded successfully", gin.H{
			"relative_path": targetRelativePath,
			"name":          sanitizedFileName,
			"size":          fileHeader.Size,
			"folder_path":   folderRelativePath,
		})
	})

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.gohtml", gin.H{
			"MongoDBURL": cfg.Databases.MongoDB.URI,
		})
	})

	router.GET("/api/bot/qr/status", func(c *gin.Context) {
		SuccessHTTPResp(c, http.StatusOK, "", eventsrunner.GetQRLoginState())
	})

	router.GET("/artifacts/process-diagram/mermaid", func(c *gin.Context) {
		brainBaseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("BRAIN_API_BASE_URL")), "/")
		if brainBaseURL == "" {
			brainBaseURL = "http://127.0.0.1:8000"
		}
		endpoint := brainBaseURL + "/api/v1/chat-agent/process-diagram"
		proxyBrainDiagram(c, endpoint)
	})

	router.GET("/artifacts/process-diagram/code-mermaid", func(c *gin.Context) {
		brainBaseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("BRAIN_API_BASE_URL")), "/")
		if brainBaseURL == "" {
			brainBaseURL = "http://127.0.0.1:8000"
		}
		endpoint := brainBaseURL + "/api/v1/chat-agent-code/process-diagram"
		proxyBrainDiagram(c, endpoint)
	})

	router.GET("/artifacts/process-diagram/research-mermaid", func(c *gin.Context) {
		brainBaseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("BRAIN_API_BASE_URL")), "/")
		if brainBaseURL == "" {
			brainBaseURL = "http://127.0.0.1:8000"
		}
		endpoint := brainBaseURL + "/api/v1/agent-research/process-diagram"
		proxyBrainDiagram(c, endpoint)
	})

	router.GET("/artifacts/process-diagram/reply-mermaid", func(c *gin.Context) {
		brainBaseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("BRAIN_API_BASE_URL")), "/")
		if brainBaseURL == "" {
			brainBaseURL = "http://127.0.0.1:8000"
		}
		endpoint := brainBaseURL + "/api/v1/agent-reply/process-diagram"
		proxyBrainDiagram(c, endpoint)
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
			"Title":                                   "Agent Accsessible Resources",
			"ProcessDiagramPath":                      "/assets/artifacts/chat_agent_process_table.png",
			"ProcessDiagramMermaidEndpoint":           "/artifacts/process-diagram/mermaid",
			"ProcessDiagramCodeMermaidEndpoint":       "/artifacts/process-diagram/code-mermaid",
			"ProcessDiagramResearchMermaidEndpoint":   "/artifacts/process-diagram/research-mermaid",
			"ProcessDiagramReplyMermaidEndpoint":      "/artifacts/process-diagram/reply-mermaid",
			"PlaygroundListEndpoint":                  "/artifacts/playground/list",
			"PlaygroundFileEndpoint":                  "/artifacts/playground/file",
			"PlaygroundOpenEndpoint":                  "/artifacts/playground/open",
			"PlaygroundDeleteEndpoint":                "/artifacts/playground/delete",
			"PlaygroundCreateFolderEndpoint":          "/artifacts/playground/folder",
			"PlaygroundUploadEndpoint":                "/artifacts/playground/upload",
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

	router.GET("/inbox", func(c *gin.Context) {
		inbox_messages, err := handler.GetInboxMessages(manager)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		inbox_address, err := manager.GetInboxAddress()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.HTML(http.StatusOK, "inbox.gohtml", gin.H{
			"InboxAddress":  inbox_address,
			"InboxMessages": inbox_messages,
			"SetupBaseURL":  fmt.Sprintf("%s%s%s", API_BASE_PATH, setupRoute.Origin, setupRoute.Inbox),
			// base on v1.POST("/message/send", sendMessageHandler(waClient)) in route.go
			"SendMessageEndpoint":        fmt.Sprintf("%s%s", BOT_ROUTE_BASE, "/message/send"),
			"InjectConversationEndpoint": fmt.Sprintf("%s%s", BOT_ROUTE_BASE, "/inject-conversation"),
		})
	})

	return router
}
