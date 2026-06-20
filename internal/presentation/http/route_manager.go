package route

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
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

	TOOL_BUILDER_PATH string = "/tool-builder"

	CATALOG_STORE_PATH string = "/catalog-store"

	ADMINS_REPLY_AGENT_PATH string = "/admin-reply-agent"

	ESCALATION_PATH string = "/escalation"
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
	Agent      string   `json:"agent"`
	FolderPath string   `json:"folder_path,omitempty"`
	URLs       []string `json:"urls,omitempty"`
	FilePaths  []string `json:"file_paths,omitempty"`
	Category   string   `json:"category"`
}

type vectorStoreURLIngestRequest struct {
	Agent    string   `json:"agent"`
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

var supportedCatalogImageExtensions = map[string]struct{}{
	".avif": {},
	".bmp":  {},
	".gif":  {},
	".heic": {},
	".jpeg": {},
	".jpg":  {},
	".png":  {},
	".svg":  {},
	".tif":  {},
	".tiff": {},
	".webp": {},
}

var supportedToolBuilderAgents = map[string]struct{}{
	"main-agent":     {},
	"code-agent":     {},
	"research-agent": {},
	"reply-agent":    {},
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

func escapeMultipartValue(value string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(value)
}

func requestBrainMultipart(ctx context.Context, method string, apiPath string, form *multipart.Form) (any, error) {
	requestCtx := ctx
	if requestCtx == nil {
		requestCtx = context.Background()
	}

	var cancel context.CancelFunc
	if _, hasDeadline := requestCtx.Deadline(); !hasDeadline {
		requestCtx, cancel = context.WithTimeout(requestCtx, brainVectorTimeout)
		defer cancel()
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for fieldName, values := range form.Value {
		for _, value := range values {
			if err := writer.WriteField(fieldName, value); err != nil {
				return nil, fmt.Errorf("write multipart field: %w", err)
			}
		}
	}

	for fieldName, fileHeaders := range form.File {
		for _, fileHeader := range fileHeaders {
			file, err := fileHeader.Open()
			if err != nil {
				return nil, fmt.Errorf("open multipart file: %w", err)
			}

			contentType := fileHeader.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "application/octet-stream"
			}

			header := textproto.MIMEHeader{}
			header.Set(
				"Content-Disposition",
				fmt.Sprintf(
					`form-data; name="%s"; filename="%s"`,
					escapeMultipartValue(fieldName),
					escapeMultipartValue(fileHeader.Filename),
				),
			)
			header.Set("Content-Type", contentType)

			part, err := writer.CreatePart(header)
			if err != nil {
				_ = file.Close()
				return nil, fmt.Errorf("create multipart part: %w", err)
			}

			if _, err := io.Copy(part, file); err != nil {
				_ = file.Close()
				return nil, fmt.Errorf("copy multipart file: %w", err)
			}
			_ = file.Close()
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(requestCtx, method, brainVectorEndpoint(apiPath), &body)
	if err != nil {
		return nil, fmt.Errorf("build brain multipart request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: brainVectorTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute brain multipart request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return nil, fmt.Errorf("brain multipart request failed: status=%d, read_body_error=%w", resp.StatusCode, readErr)
		}

		bodyText := strings.TrimSpace(string(bodyBytes))
		if bodyText == "" {
			bodyText = fmt.Sprintf("upstream status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("brain multipart request failed: status=%d, body=%s", resp.StatusCode, bodyText)
	}

	var responsePayload any
	if err := json.NewDecoder(resp.Body).Decode(&responsePayload); err != nil {
		return nil, fmt.Errorf("decode brain multipart response: %w", err)
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

func normalizeVectorAgent(rawAgent string) string {
	return strings.TrimSpace(rawAgent)
}

func requireVectorAgent(rawAgent string) (string, error) {
	agent := normalizeVectorAgent(rawAgent)
	if agent == "" {
		return "", fmt.Errorf("agent is required")
	}
	return agent, nil
}

func requireToolBuilderAgent(rawAgent string) (string, error) {
	agent := strings.TrimSpace(rawAgent)
	if agent == "" {
		return "", fmt.Errorf("agent is required")
	}
	if _, ok := supportedToolBuilderAgents[agent]; !ok {
		return "", fmt.Errorf("unsupported agent: %s", agent)
	}
	return agent, nil
}

func toolBuilderRequestToDomain(req ToolBuilderRequest) ports.ToolBuilderData {
	tool := ports.ToolBuilderData{
		Name:        req.Name,
		Description: req.Description,
		AgentType:   req.AgentType,
		Information: req.Information,
	}
	if req.Code != nil {
		tool.Code = &ports.ToolDataCode{
			Package: req.Code.Package,
			Code:    req.Code.Code,
		}
	}
	return tool
}

func toolBuilderErrorHTTPResp(c *gin.Context, err error, defaultMessage string) {
	switch {
	case errors.Is(err, ports.ErrInvalidToolBuilderData):
		ErrorHTTPResp(c, http.StatusBadRequest, "name, description, agent_type, and one of information or code are required")
	case errors.Is(err, ports.ErrToolBuilderAlreadyExists):
		ErrorHTTPResp(c, http.StatusConflict, "tool builder already exists for this agent")
	case errors.Is(err, ports.ErrToolBuilderNotFound):
		ErrorHTTPResp(c, http.StatusNotFound, "tool builder not found")
	default:
		ErrorHTTPResp(c, http.StatusInternalServerError, defaultMessage)
	}
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
	agent, err := requireVectorAgent(ingestRequest.Agent)
	if err != nil {
		return nil, err
	}
	ingestRequest.Agent = agent
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
		agent, err := requireVectorAgent(c.Query("agent"))
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, err.Error())
			return
		}

		query := url.Values{}
		query.Set("agent", agent)
		payload, err := requestBrainJSON(c.Request.Context(), http.MethodGet, "/vector-store/documents", query, nil)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to get vector documents: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "vector documents retrieved successfully", payload)
	}
}

func getSupportedAgentForVectorDataHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		payload, err := requestBrainJSON(c.Request.Context(), http.MethodGet, "/vector-store/supported-agent", nil, nil)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to get supported agent for vector data: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "supported agent for vector data retrieved successfully", payload)
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

		agent, err := requireVectorAgent(c.PostForm("agent"))
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, err.Error())
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
			Agent:     agent,
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

		agent, err := requireVectorAgent(req.Agent)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, err.Error())
			return
		}

		payload, err := ingestVectorDocuments(c.Request.Context(), vectorStoreIngestRequest{
			Agent:    agent,
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
		agent, err := requireVectorAgent(c.Query("agent"))
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, err.Error())
			return
		}

		query := url.Values{}
		query.Set("agent", agent)
		query.Set("source", source)
		payload, err := requestBrainJSON(c.Request.Context(), http.MethodDelete, "/vector-store/documents", query, nil)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to delete vector document: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "vector document deleted successfully", payload)
	}
}

func getToolsBuilderListHandler(manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		agent, err := requireToolBuilderAgent(c.Query("agent"))
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, err.Error())
			return
		}

		tools, err := handler.GetToolsBuilderList(manager, agent)
		if err != nil {
			toolBuilderErrorHTTPResp(c, err, "failed to get tool builder list")
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "tool builder list retrieved successfully", tools)
	}
}

func addToolBuilderHandler(manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ToolBuilderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		agent, err := requireToolBuilderAgent(req.AgentType)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, err.Error())
			return
		}
		req.AgentType = agent

		if err := handler.AddToolBuilder(manager, toolBuilderRequestToDomain(req)); err != nil {
			toolBuilderErrorHTTPResp(c, err, "failed to add tool builder")
			return
		}

		SuccessHTTPResp(c, http.StatusCreated, "tool builder added successfully", nil)
	}
}

func updateToolBuilderHandler(manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ToolBuilderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		agent, err := requireToolBuilderAgent(req.AgentType)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, err.Error())
			return
		}
		originalName := strings.TrimSpace(req.OriginalName)
		if originalName == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "original_name is required")
			return
		}
		req.AgentType = agent

		if err := handler.UpdateToolBuilder(manager, agent, originalName, toolBuilderRequestToDomain(req)); err != nil {
			toolBuilderErrorHTTPResp(c, err, "failed to update tool builder")
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "tool builder updated successfully", nil)
	}
}

func deleteToolBuilderHandler(manager ports.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req DeleteToolBuilderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		agent, err := requireToolBuilderAgent(req.AgentType)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, err.Error())
			return
		}

		if err := handler.DeleteToolBuilder(manager, agent, req.Name); err != nil {
			toolBuilderErrorHTTPResp(c, err, "failed to delete tool builder")
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "tool builder deleted successfully", nil)
	}
}

func catalogProductPath(catalogID string) string {
	return "/catalog/" + url.PathEscape(catalogID)
}

func isCatalogImageUpload(fileHeader *multipart.FileHeader) bool {
	contentType := strings.ToLower(strings.TrimSpace(fileHeader.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "image/") {
		return true
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	_, ok := supportedCatalogImageExtensions[ext]
	return ok
}

func getCatalogProductsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		query := c.Request.URL.Query()
		apiPath := "/catalog"
		if len(query["ids"]) > 0 {
			apiPath = "/catalog/by-ids"
		}

		payload, err := requestBrainJSON(c.Request.Context(), http.MethodGet, apiPath, query, nil)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to get catalog products: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "catalog products retrieved successfully", payload)
	}
}

func getCatalogProductHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		catalogID := strings.TrimSpace(c.Param("id"))
		if catalogID == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "catalog id is required")
			return
		}

		payload, err := requestBrainJSON(c.Request.Context(), http.MethodGet, catalogProductPath(catalogID), nil, nil)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to get catalog product: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "catalog product retrieved successfully", payload)
	}
}

func createCatalogProductHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req map[string]any
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		payload, err := requestBrainJSON(c.Request.Context(), http.MethodPost, "/catalog", nil, req)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to create catalog product: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusCreated, "catalog product created successfully", payload)
	}
}

func updateCatalogProductHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		catalogID := strings.TrimSpace(c.Param("id"))
		if catalogID == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "catalog id is required")
			return
		}

		var req map[string]any
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		payload, err := requestBrainJSON(c.Request.Context(), http.MethodPut, catalogProductPath(catalogID), nil, req)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to update catalog product: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "catalog product updated successfully", payload)
	}
}

func deleteCatalogProductHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		catalogID := strings.TrimSpace(c.Param("id"))
		if catalogID == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "catalog id is required")
			return
		}

		payload, err := requestBrainJSON(c.Request.Context(), http.MethodDelete, catalogProductPath(catalogID), nil, nil)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to delete catalog product: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "catalog product deleted successfully", payload)
	}
}

func getCatalogImagesHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		catalogID := strings.TrimSpace(c.Param("id"))
		if catalogID == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "catalog id is required")
			return
		}

		payload, err := requestBrainJSON(c.Request.Context(), http.MethodGet, catalogProductPath(catalogID)+"/images", nil, nil)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to get catalog images: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "catalog images retrieved successfully", payload)
	}
}

func addCatalogImageURLHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		catalogID := strings.TrimSpace(c.Param("id"))
		if catalogID == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "catalog id is required")
			return
		}

		var req CatalogImageURLRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		payload, err := requestBrainJSON(c.Request.Context(), http.MethodPost, catalogProductPath(catalogID)+"/images/url", nil, req)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to add catalog image URL: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusCreated, "catalog image URL added successfully", payload)
	}
}

func uploadCatalogImagesHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		catalogID := strings.TrimSpace(c.Param("id"))
		if catalogID == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "catalog id is required")
			return
		}

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

		for _, fileHeader := range fileHeaders {
			if !isCatalogImageUpload(fileHeader) {
				ErrorHTTPResp(c, http.StatusBadRequest, "uploaded catalog files must be images")
				return
			}
		}

		proxyForm := &multipart.Form{
			Value: form.Value,
			File: map[string][]*multipart.FileHeader{
				"files": fileHeaders,
			},
		}
		payload, err := requestBrainMultipart(c.Request.Context(), http.MethodPost, catalogProductPath(catalogID)+"/images/upload", proxyForm)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to upload catalog images: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusCreated, "catalog images uploaded successfully", payload)
	}
}

func deleteCatalogImageHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		catalogID := strings.TrimSpace(c.Param("id"))
		imageID := strings.TrimSpace(c.Param("image_id"))
		if catalogID == "" || imageID == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "catalog id and image id are required")
			return
		}

		endpoint := catalogProductPath(catalogID) + "/images/" + url.PathEscape(imageID)
		payload, err := requestBrainJSON(c.Request.Context(), http.MethodDelete, endpoint, nil, nil)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to delete catalog image: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "catalog image deleted successfully", payload)
	}
}

func replyAdminPath(number string) string {
	if strings.TrimSpace(number) == "" {
		return "/admin/number-store"
	}
	return "/admin/number-store/" + url.PathEscape(number)
}

func getReplyAdminsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		payload, err := requestBrainJSON(c.Request.Context(), http.MethodGet, replyAdminPath(""), nil, nil)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to get reply admins: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "reply admins retrieved successfully", payload)
	}
}

func getReplyAdminHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		number := strings.TrimSpace(c.Param("number"))
		if number == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "admin number is required")
			return
		}

		payload, err := requestBrainJSON(c.Request.Context(), http.MethodGet, replyAdminPath(number), nil, nil)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to get reply admin: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "reply admin retrieved successfully", payload)
	}
}

func createReplyAdminHandler(waClient ports.WhatsAppClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ReplyAdminRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		contact, err := waClient.GetContact(c.Request.Context(), req.Number)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "failed to get contact information")
			return
		}

		if !contact.Found {
			ErrorHTTPResp(c, http.StatusBadRequest, "the provided number is not a valid WhatsApp contact")
			return
		}

		payload, err := requestBrainJSON(c.Request.Context(), http.MethodPost, replyAdminPath(""), nil, req)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to create reply admin: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusCreated, "reply admin created successfully", payload)
	}
}

func updateReplyAdminHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		number := strings.TrimSpace(c.Param("number"))
		if number == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "admin number is required")
			return
		}

		var req ReplyAdminRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}

		payload, err := requestBrainJSON(c.Request.Context(), http.MethodPut, replyAdminPath(number), nil, req)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to update reply admin: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "reply admin updated successfully", payload)
	}
}

func deleteReplyAdminHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		number := strings.TrimSpace(c.Param("number"))
		if number == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "admin number is required")
			return
		}

		payload, err := requestBrainJSON(c.Request.Context(), http.MethodDelete, replyAdminPath(number), nil, nil)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to delete reply admin: "+err.Error())
			return
		}

		SuccessHTTPResp(c, http.StatusOK, "reply admin deleted successfully", payload)
	}
}

func cleanIdentifier(identifier string) string {
	cleaned := strings.TrimSpace(identifier)
	for strings.HasPrefix(cleaned, "\"") && strings.HasSuffix(cleaned, "\"") {
		cleaned = strings.Trim(cleaned, "\"")
		cleaned = strings.TrimSpace(cleaned)
	}
	return cleaned
}

func escalationPath(subPath string) string {
	return "/escalation" + subPath
}

func getEscalationWaitingListHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		payload, err := requestBrainJSON(c.Request.Context(), http.MethodGet, escalationPath("/waiting"), nil, nil)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to get escalation waiting list: "+err.Error())
			return
		}
		SuccessHTTPResp(c, http.StatusOK, "escalation waiting list retrieved successfully", payload)
	}
}

func getEscalationWaitingHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		identifier := cleanIdentifier(c.Param("identifier"))
		if identifier == "" {
			ErrorHTTPResp(c, http.StatusBadRequest, "identifier is required")
			return
		}
		payload, err := requestBrainJSON(c.Request.Context(), http.MethodGet, escalationPath("/waiting/"+url.PathEscape(identifier)), nil, nil)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to get escalation waiting status: "+err.Error())
			return
		}
		SuccessHTTPResp(c, http.StatusOK, "escalation waiting status retrieved successfully", payload)
	}
}

func disableAIReplyHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req EscalationRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}
		req.Identifier = cleanIdentifier(req.Identifier)
		payload, err := requestBrainJSON(c.Request.Context(), http.MethodPost, escalationPath("/disable"), nil, req)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to disable AI reply: "+err.Error())
			return
		}
		SuccessHTTPResp(c, http.StatusOK, "AI reply disabled successfully", payload)
	}
}

func enableAIReplyHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req EscalationRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ErrorHTTPResp(c, http.StatusBadRequest, "invalid request body")
			return
		}
		req.Identifier = cleanIdentifier(req.Identifier)
		payload, err := requestBrainJSON(c.Request.Context(), http.MethodPost, escalationPath("/enable"), nil, req)
		if err != nil {
			ErrorHTTPResp(c, http.StatusBadGateway, "failed to enable AI reply: "+err.Error())
			return
		}
		SuccessHTTPResp(c, http.StatusOK, "AI reply enabled successfully", payload)
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
			load_doc_to_vector.GET("/supported-agent", getSupportedAgentForVectorDataHandler())
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

		toolBuilder := r.Group(TOOL_BUILDER_PATH)
		{
			toolBuilder.GET("/tools", getToolsBuilderListHandler(manager))
			toolBuilder.POST("/tools", addToolBuilderHandler(manager))
			toolBuilder.PUT("/tools", updateToolBuilderHandler(manager))
			toolBuilder.DELETE("/tools", deleteToolBuilderHandler(manager))
		}

		catalogStore := r.Group(CATALOG_STORE_PATH)
		{
			catalogStore.GET("/products", getCatalogProductsHandler())
			catalogStore.POST("/products", createCatalogProductHandler())
			catalogStore.GET("/products/:id", getCatalogProductHandler())
			catalogStore.PUT("/products/:id", updateCatalogProductHandler())
			catalogStore.DELETE("/products/:id", deleteCatalogProductHandler())
			catalogStore.GET("/products/:id/images", getCatalogImagesHandler())
			catalogStore.POST("/products/:id/images/url", addCatalogImageURLHandler())
			catalogStore.POST("/products/:id/images/upload", uploadCatalogImagesHandler())
			catalogStore.DELETE("/products/:id/images/:image_id", deleteCatalogImageHandler())
		}

		admins_reply_agent := r.Group(ADMINS_REPLY_AGENT_PATH)
		{
			admins_reply_agent.GET("/admins", getReplyAdminsHandler())
			admins_reply_agent.POST("/admins", createReplyAdminHandler(waClient))
			admins_reply_agent.GET("/admins/:number", getReplyAdminHandler())
			admins_reply_agent.PUT("/admins/:number", updateReplyAdminHandler())
			admins_reply_agent.DELETE("/admins/:number", deleteReplyAdminHandler())
		}

		escalation := r.Group(ESCALATION_PATH)
		{
			escalation.GET("/waiting", getEscalationWaitingListHandler())
			escalation.GET("/waiting/:identifier", getEscalationWaitingHandler())
			escalation.POST("/disable", disableAIReplyHandler())
			escalation.POST("/enable", enableAIReplyHandler())
		}
		

	}
	return router
}
