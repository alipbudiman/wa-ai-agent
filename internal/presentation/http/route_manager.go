package route

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/alipbudiman/wa-ai-agent.git/internal/config"
	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"github.com/alipbudiman/wa-ai-agent.git/internal/presentation/handler"
	"github.com/gin-gonic/gin"
)

const (
	API_BASE_PATH          string = "/api"
	ACCESS_PATH            string = "/access"
	REPORTS_PATH           string = "/reports"
	BOT_SETUP_PATH         string = "/bot-setup"
	LOAD_DOCUMENT_PATH     string = "/load-document"
	defaultDocumentRootDir string = "uploads/documents"
	brainVectorTimeout            = 360 * time.Second
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

type vectorStoreIngestRequest struct {
	FolderPath string   `json:"folder_path,omitempty"`
	URLs       []string `json:"urls,omitempty"`
	FilePaths  []string `json:"file_paths,omitempty"`
	Category   string   `json:"category"`
}

type vectorStoreURLIngestRequest struct {
	URLs     []string `json:"urls" binding:"required"`
	Category string   `json:"category"`
}

type vectorStoreUploadResult struct {
	SavedFiles     []string `json:"saved_files"`
	IngestResponse any      `json:"ingest_response,omitempty"`
}

var supportedVectorDocumentExtensions = map[string]struct{}{
	".pdf":  {},
	".txt":  {},
	".md":   {},
	".csv":  {},
	".docx": {},
	".xlsx": {},
	".pptx": {},
}

func brainBaseURL() string {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("BRAIN_API_BASE_URL")), "/")
	if baseURL == "" {
		return "http://127.0.0.1:8000"
	}
	return baseURL
}

func documentUploadRootDir() string {
	rootDir := strings.TrimSpace(os.Getenv("DOCUMENT_STORAGE_DIR"))
	if rootDir == "" {
		return defaultDocumentRootDir
	}
	return rootDir
}

func brainVectorEndpoint(apiPath string) string {
	baseURL := brainBaseURL()
	if strings.HasSuffix(baseURL, "/api/v1") {
		return baseURL + apiPath
	}
	return baseURL + "/api/v1" + apiPath
}

func requestBrainJSON(ctx context.Context, method string, apiPath string, query url.Values, payload any) (any, error) {
	requestCtx := ctx
	if requestCtx == nil {
		requestCtx = context.Background()
	}

	var cancel context.CancelFunc
	if _, hasDeadline := requestCtx.Deadline(); !hasDeadline {
		requestCtx, cancel = context.WithTimeout(requestCtx, brainVectorTimeout)
		defer cancel()
	}

	var body io.Reader
	if payload != nil {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal brain payload: %w", err)
		}
		body = bytes.NewReader(payloadBytes)
	}

	endpoint := brainVectorEndpoint(apiPath)
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(requestCtx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("build brain request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: brainVectorTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute brain request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return nil, fmt.Errorf("brain request failed: status=%d, read_body_error=%w", resp.StatusCode, readErr)
		}

		bodyText := strings.TrimSpace(string(bodyBytes))
		if bodyText == "" {
			bodyText = fmt.Sprintf("upstream status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("brain request failed: status=%d, body=%s", resp.StatusCode, bodyText)
	}

	var responsePayload any
	if err := json.NewDecoder(resp.Body).Decode(&responsePayload); err != nil {
		return nil, fmt.Errorf("decode brain response: %w", err)
	}

	return responsePayload, nil
}

func normalizeVectorCategory(rawCategory string) string {
	category := strings.TrimSpace(rawCategory)
	if category == "" {
		return "general"
	}
	return category
}

func validateVectorDocumentName(fileName string) (string, error) {
	sanitizedName, err := normalizePlaygroundNodeName(path.Base(strings.ReplaceAll(fileName, "\\", "/")))
	if err != nil {
		return "", err
	}

	ext := strings.ToLower(filepath.Ext(sanitizedName))
	if _, ok := supportedVectorDocumentExtensions[ext]; !ok {
		return "", fmt.Errorf("unsupported document type: %s", ext)
	}

	return sanitizedName, nil
}

func resolveStoredDocumentPath(rawPath string) (string, error) {
	source := strings.TrimSpace(rawPath)
	if source == "" {
		return "", fmt.Errorf("source is required")
	}

	documentRootAbs, err := filepath.Abs(documentUploadRootDir())
	if err != nil {
		return "", err
	}

	sourceAbs, err := filepath.Abs(source)
	if err != nil {
		return "", err
	}

	relativePath, err := filepath.Rel(documentRootAbs, sourceAbs)
	if err != nil {
		return "", err
	}

	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("source is outside document storage")
	}

	return sourceAbs, nil
}

func ingestVectorDocuments(ctx context.Context, ingestRequest vectorStoreIngestRequest) (any, error) {
	ingestRequest.Category = normalizeVectorCategory(ingestRequest.Category)
	return requestBrainJSON(ctx, http.MethodPost, "/vector-store/ingest-document", nil, ingestRequest)
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

func getVectorDocumentsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		payload, err := requestBrainJSON(c.Request.Context(), http.MethodGet, "/vector-store/documents", nil, nil)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to get vector documents: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "vector documents retrieved successfully", payload)
	}
}

func getSupportedVectorDocumentTypeHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		payload, err := requestBrainJSON(c.Request.Context(), http.MethodGet, "/vector-store/supported-document-type", nil, nil)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to get supported document types: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "supported document types retrieved successfully", payload)
	}
}

func uploadVectorDocumentsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		form, err := c.MultipartForm()
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid multipart form")
			return
		}

		fileHeaders := form.File["files"]
		if len(fileHeaders) == 0 {
			if singleFile, err := c.FormFile("file"); err == nil {
				fileHeaders = append(fileHeaders, singleFile)
			}
		}
		if len(fileHeaders) == 0 {
			ErrorHTTPResp(c, http.StatusBadRequest, "files are required")
			return
		}

		requestID := fmt.Sprintf("%d", time.Now().UnixNano())
		documentDir := filepath.Join(documentUploadRootDir(), requestID)
		documentDirAbs, err := filepath.Abs(documentDir)
		if err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to resolve upload directory")
			return
		}

		if err := os.MkdirAll(documentDirAbs, 0o755); err != nil {
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to create document upload directory")
			return
		}

		savedFiles := make([]string, 0, len(fileHeaders))
		usedNames := make(map[string]int)
		for _, fileHeader := range fileHeaders {
			originalSafeName, err := validateVectorDocumentName(fileHeader.Filename)
			if err != nil {
				_ = os.RemoveAll(documentDirAbs)
				ErrorHTTPResp(c, http.StatusBadRequest, err.Error())
				return
			}

			safeName := originalSafeName
			if count := usedNames[originalSafeName]; count > 0 {
				ext := filepath.Ext(safeName)
				baseName := strings.TrimSuffix(safeName, ext)
				safeName = fmt.Sprintf("%s-%d%s", baseName, count+1, ext)
			}
			usedNames[originalSafeName]++

			targetPath := filepath.Join(documentDirAbs, safeName)
			if err := c.SaveUploadedFile(fileHeader, targetPath); err != nil {
				_ = os.RemoveAll(documentDirAbs)
				ErrorHTTPResp(c, http.StatusInternalServerError, "failed to save uploaded file")
				return
			}

			absPath, err := filepath.Abs(targetPath)
			if err != nil {
				_ = os.RemoveAll(documentDirAbs)
				ErrorHTTPResp(c, http.StatusInternalServerError, "failed to resolve uploaded file path")
				return
			}
			savedFiles = append(savedFiles, absPath)
		}

		ingestPayload, ingestErr := ingestVectorDocuments(c.Request.Context(), vectorStoreIngestRequest{
			FilePaths: savedFiles,
			Category:  c.PostForm("category"),
		})

		if ingestErr != nil {
			_ = os.RemoveAll(documentDirAbs)
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to ingest uploaded documents: "+ingestErr.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "documents uploaded, ingested, and stored", vectorStoreUploadResult{
			SavedFiles:     savedFiles,
			IngestResponse: ingestPayload,
		})
	}
}

func ingestVectorURLsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req vectorStoreURLIngestRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		urls := make([]string, 0, len(req.URLs))
		for _, rawURL := range req.URLs {
			trimmedURL := strings.TrimSpace(rawURL)
			if trimmedURL != "" {
				urls = append(urls, trimmedURL)
			}
		}
		if len(urls) == 0 {
			ErrorHTTPResp(c, http.StatusBadRequest, "urls are required")
			return
		}

		payload, err := ingestVectorDocuments(c.Request.Context(), vectorStoreIngestRequest{
			URLs:     urls,
			Category: req.Category,
		})
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to ingest urls: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "urls ingested successfully", payload)
	}
}

func viewVectorDocumentHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sourcePath, err := resolveStoredDocumentPath(c.Query("source"))
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, err.Error())
			return
		}

		fileInfo, err := os.Stat(sourcePath)
		if err != nil {
			if os.IsNotExist(err) {
				ErrorHTTPResp(c, http.StatusNotFound, "document file not found")
				return
			}
			ErrorHTTPResp(c, http.StatusInternalServerError, "failed to inspect document file")
			return
		}

		if fileInfo.IsDir() {
			ErrorHTTPResp(c, http.StatusBadRequest, "source points to a folder")
			return
		}

		c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", filepath.Base(sourcePath)))
		c.File(sourcePath)
	}
}

func deleteVectorDocumentHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		source := strings.TrimSpace(c.Query("source"))
		if source == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "source query parameter is required")
			return
		}

		query := url.Values{}
		query.Set("source", source)
		payload, err := requestBrainJSON(c.Request.Context(), http.MethodDelete, "/vector-store/documents", query, nil)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to delete vector document: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "vector document deleted successfully", payload)
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

		load_doc_to_vector := r.Group(LOAD_DOCUMENT_PATH)
		{
			load_doc_to_vector.GET("/documents", getVectorDocumentsHandler())
			load_doc_to_vector.GET("/supported-document-type", getSupportedVectorDocumentTypeHandler())
			load_doc_to_vector.GET("/file", viewVectorDocumentHandler())
			load_doc_to_vector.POST("/upload", uploadVectorDocumentsHandler())
			load_doc_to_vector.POST("/ingest-urls", ingestVectorURLsHandler())
			load_doc_to_vector.DELETE("/documents", deleteVectorDocumentHandler())
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
