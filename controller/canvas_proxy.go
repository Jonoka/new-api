package controller

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func CanvasPrepareRequest(c *gin.Context) {
	group := strings.TrimSpace(c.Query("group"))
	if group == "" {
		abortCanvasRequest(c, http.StatusBadRequest, "group is required")
		return
	}

	userID := c.GetInt("id")
	userCache, err := model.GetUserCache(userID)
	if err != nil {
		abortCanvasRequest(c, http.StatusInternalServerError, "failed to load user")
		return
	}
	if !service.GroupInUserUsableGroups(userCache.Group, group) && group != userCache.Group {
		abortCanvasRequest(c, http.StatusForbidden, fmt.Sprintf("无权访问 %s 分组", group))
		return
	}

	userCache.WriteContext(c)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, group)

	tempToken := &model.Token{
		UserId: userID,
		Name:   fmt.Sprintf("canvas-%s", group),
		Group:  group,
	}
	if err := middleware.SetupContextForToken(c, tempToken); err != nil {
		abortCanvasRequest(c, http.StatusInternalServerError, err.Error())
		return
	}

	if c.Request.Method != http.MethodGet {
		if err := injectCanvasGroup(c); err != nil {
			abortCanvasRequest(c, http.StatusBadRequest, err.Error())
			return
		}
	}

	c.Next()
}

func CanvasListModels(c *gin.Context) {
	ListModels(c, constant.ChannelTypeOpenAI)
}

func abortCanvasRequest(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"error": types.OpenAIError{
			Message: message,
			Type:    "invalid_request_error",
		},
	})
	c.Abort()
}

func injectCanvasGroup(c *gin.Context) error {
	group := strings.TrimSpace(c.Query("group"))
	if group == "" {
		return errors.New("group is required")
	}

	contentType := c.Request.Header.Get("Content-Type")
	switch {
	case strings.HasPrefix(contentType, "application/json") || contentType == "":
		return injectCanvasJSONGroup(c, group)
	case strings.HasPrefix(contentType, "multipart/form-data"):
		return injectCanvasMultipartGroup(c, group, contentType)
	default:
		return nil
	}
}

func injectCanvasJSONGroup(c *gin.Context, group string) error {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}
	_ = c.Request.Body.Close()
	if len(bytes.TrimSpace(body)) == 0 {
		body = []byte("{}")
	}

	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return err
	}
	payload["group"] = group

	nextBody, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	setCanvasRequestBody(c, nextBody)
	return nil
}

func injectCanvasMultipartGroup(c *gin.Context, group string, contentType string) error {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return err
	}
	reader := multipart.NewReader(c.Request.Body, params["boundary"])
	form, err := reader.ReadForm(int64(constant.MaxRequestBodyMB) << 20)
	_ = c.Request.Body.Close()
	if err != nil {
		return err
	}
	defer form.RemoveAll()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("group", group); err != nil {
		return err
	}
	for key, values := range form.Value {
		if key == "group" {
			continue
		}
		for _, value := range values {
			if err := writer.WriteField(key, value); err != nil {
				return err
			}
		}
	}
	for key, files := range form.File {
		for _, fileHeader := range files {
			if err := copyMultipartFile(writer, key, fileHeader); err != nil {
				return err
			}
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}

	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	setCanvasRequestBody(c, body.Bytes())
	return nil
}

func copyMultipartFile(writer *multipart.Writer, field string, fileHeader *multipart.FileHeader) error {
	file, err := fileHeader.Open()
	if err != nil {
		return err
	}
	defer file.Close()

	part, err := writer.CreateFormFile(field, fileHeader.Filename)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	return err
}

func setCanvasRequestBody(c *gin.Context, body []byte) {
	storage, err := common.CreateBodyStorage(body)
	if err == nil {
		c.Set(common.KeyBodyStorage, storage)
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.ContentLength = int64(len(body))
	c.Request.Header.Set("Content-Length", fmt.Sprint(len(body)))
}
