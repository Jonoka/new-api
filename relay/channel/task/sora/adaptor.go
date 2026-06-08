package sora

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/imageutil"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/tidwall/sjson"
)

// ============================
// Request / Response structures
// ============================

type ContentItem struct {
	Type     string    `json:"type"`                // "text" or "image_url"
	Text     string    `json:"text,omitempty"`      // for text type
	ImageURL *ImageURL `json:"image_url,omitempty"` // for image_url type
}

type ImageURL struct {
	URL string `json:"url"`
}

type responseTask struct {
	ID                 string `json:"id"`
	TaskID             string `json:"task_id,omitempty"` //兼容旧接口
	Object             string `json:"object"`
	Model              string `json:"model"`
	Status             string `json:"status"`
	Progress           int    `json:"progress"`
	CreatedAt          int64  `json:"created_at"`
	CompletedAt        int64  `json:"completed_at,omitempty"`
	ExpiresAt          int64  `json:"expires_at,omitempty"`
	Seconds            string `json:"seconds,omitempty"`
	Size               string `json:"size,omitempty"`
	RemixedFromVideoID string `json:"remixed_from_video_id,omitempty"`
	Error              *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) PrepareBillingRequestInput(c *gin.Context, info *relaycommon.RelayInfo) error {
	if !shouldUseChannel25VideoMapping(info) {
		return nil
	}
	bodyMap, ok, err := buildChannel25VideoMappedBodyForBilling(c, info)
	if err != nil || !ok {
		return err
	}
	bodyBytes, err := common.Marshal(bodyMap)
	if err != nil {
		return err
	}
	headers := map[string]string{"Content-Type": "application/json"}
	if info != nil {
		for k, v := range info.RequestHeaders {
			headers[k] = v
		}
	}
	headers["Content-Type"] = "application/json"
	info.BillingRequestInput = &billingexpr.RequestInput{
		Headers: headers,
		Body:    bodyBytes,
	}
	return nil
}

func validateRemixRequest(c *gin.Context) *dto.TaskError {
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("field prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	// 存储原始请求到 context，与 ValidateMultipartDirect 路径保持一致
	c.Set("task_request", req)
	return nil
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	if info.Action == constant.TaskActionRemix {
		return validateRemixRequest(c)
	}
	return relaycommon.ValidateMultipartDirect(c, info)
}

// EstimateBilling 根据用户请求的 seconds 和 size 计算 OtherRatios。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	// remix 路径的 OtherRatios 已在 ResolveOriginTask 中设置
	if info.Action == constant.TaskActionRemix {
		return nil
	}

	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	seconds, _ := strconv.Atoi(req.Seconds)
	if seconds == 0 {
		seconds = req.Duration
	}
	if seconds <= 0 {
		seconds = 4
	}

	size := req.Size
	if size == "" {
		size = "720x1280"
	}

	ratios := map[string]float64{
		"seconds": float64(seconds),
		"size":    1,
	}
	if size == "1792x1024" || size == "1024x1792" {
		ratios["size"] = 1.666667
	}
	return ratios
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	base := strings.TrimRight(a.baseURL, "/")
	requestPath := info.RequestURLPath
	if requestPath == "" {
		requestPath = "/v1/videos"
	}
	requestPath = strings.Split(requestPath, "?")[0]

	if info.Action == constant.TaskActionRemix {
		return fmt.Sprintf("%s/v1/videos/%s/remix", base, info.OriginTaskID), nil
	}
	if shouldUseGPT2APIVideoGenerations(info) || strings.HasPrefix(requestPath, "/v1/video/generations") {
		return base + "/v1/video/generations", nil
	}
	return fmt.Sprintf("%s/v1/videos", base), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	if shouldUseGPT2APIVideoGenerations(info) {
		req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
		return nil
	}
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_request_body_failed")
	}
	cachedBody, err := storage.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "read_body_bytes_failed")
	}
	contentType := c.GetHeader("Content-Type")

	if strings.HasPrefix(contentType, "application/json") {
		var bodyMap map[string]interface{}
		if err := common.Unmarshal(cachedBody, &bodyMap); err == nil {
			bodyMap["model"] = channel25VideoModelForRequest(info, bodyMap)
			if shouldUseGPT2APIVideoGenerations(info) {
				bodyMap["model"] = info.UpstreamModelName
				mapGPT2APIVideoJSONBody(bodyMap)
			} else if shouldUseChannel25VideoMapping(info) {
				mapChannel25VideoJSONBody(bodyMap)
			}
			if newBody, err := common.Marshal(bodyMap); err == nil {
				return bytes.NewReader(newBody), nil
			}
		}
		return bytes.NewReader(cachedBody), nil
	}

	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") && shouldUseGPT2APIVideoGenerations(info) {
		if values, err := parseURLEncodedForm(cachedBody); err == nil {
			bodyMap := formValuesToMap(values)
			bodyMap["model"] = info.UpstreamModelName
			mapGPT2APIVideoJSONBody(bodyMap)
			if newBody, err := common.Marshal(bodyMap); err == nil {
				c.Request.Header.Set("Content-Type", "application/json")
				return bytes.NewReader(newBody), nil
			}
		}
	}

	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") && shouldUseChannel25VideoMapping(info) {
		if values, err := parseURLEncodedForm(cachedBody); err == nil {
			bodyMap := formValuesToMap(values)
			bodyMap["model"] = channel25VideoModelForRequest(info, bodyMap)
			mapChannel25VideoJSONBody(bodyMap)
			if newBody, err := common.Marshal(bodyMap); err == nil {
				c.Request.Header.Set("Content-Type", "application/json")
				return bytes.NewReader(newBody), nil
			}
		}
	}

	if strings.Contains(contentType, "multipart/form-data") {
		formData, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return bytes.NewReader(cachedBody), nil
		}
		gpt2apiVideo := shouldUseGPT2APIVideoGenerations(info)
		channel25Video := shouldUseChannel25VideoMapping(info)
		if gpt2apiVideo {
			bodyMap := formValuesToMap(formData.Value)
			bodyMap["model"] = info.UpstreamModelName
			if err := addGPT2APIVideoInputReferenceFiles(bodyMap, formData.File); err != nil {
				return nil, err
			}
			mapGPT2APIVideoJSONBody(bodyMap)
			if newBody, err := common.Marshal(bodyMap); err == nil {
				c.Request.Header.Set("Content-Type", "application/json")
				return bytes.NewReader(newBody), nil
			}
		}
		if channel25Video {
			bodyMap := formValuesToMap(formData.Value)
			bodyMap["model"] = channel25VideoModelForRequest(info, bodyMap)
			if err := addChannel25VideoReferenceFiles(bodyMap, formData.File); err != nil {
				return nil, err
			}
			mapChannel25VideoJSONBody(bodyMap)
			if newBody, err := common.Marshal(bodyMap); err == nil {
				c.Request.Header.Set("Content-Type", "application/json")
				return bytes.NewReader(newBody), nil
			}
		}
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		writer.WriteField("model", info.UpstreamModelName)
		if gpt2apiVideo {
			writeGPT2APIVideoMappedFields(writer, formData.Value)
		} else {
			for key, values := range formData.Value {
				if key == "model" {
					continue
				}
				for _, v := range values {
					writer.WriteField(key, v)
				}
			}
		}
		for fieldName, fileHeaders := range formData.File {
			upstreamFieldName := fieldName
			if gpt2apiVideo && fieldName == "input_reference[]" {
				upstreamFieldName = "image"
			}
			for _, fh := range fileHeaders {
				f, err := fh.Open()
				if err != nil {
					continue
				}
				ct := fh.Header.Get("Content-Type")
				if ct == "" || ct == "application/octet-stream" {
					buf512 := make([]byte, 512)
					n, _ := io.ReadFull(f, buf512)
					ct = http.DetectContentType(buf512[:n])
					// Re-open after sniffing so the full content is copied below
					f.Close()
					f, err = fh.Open()
					if err != nil {
						continue
					}
				}
				h := make(textproto.MIMEHeader)
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, upstreamFieldName, fh.Filename))
				h.Set("Content-Type", ct)
				part, err := writer.CreatePart(h)
				if err != nil {
					f.Close()
					continue
				}
				io.Copy(part, f)
				f.Close()
			}
		}
		writer.Close()
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return &buf, nil
	}

	return common.ReaderOnly(storage), nil
}

func shouldUseGPT2APIVideoGenerations(info *relaycommon.RelayInfo) bool {
	if info == nil || info.Action == constant.TaskActionRemix {
		return false
	}
	base := strings.ToLower(strings.TrimSpace(info.ChannelBaseUrl))
	modelName := strings.ToLower(strings.TrimSpace(info.UpstreamModelName))
	requestPath := strings.Split(strings.TrimSpace(info.RequestURLPath), "?")[0]
	if !strings.HasPrefix(requestPath, "/v1/videos") {
		return false
	}
	if info.ChannelId == 23 || strings.Contains(base, "gpt2api.com") {
		return isGPT2APIVideoGenerationsModel(modelName)
	}
	return false
}

func isGPT2APIVideoGenerationsModel(modelName string) bool {
	switch modelName {
	case "grok-imagine-video", "sora", "veo3.1", "veo3.1-flash", "veo3.1-lite":
		return true
	default:
		return false
	}
}

func shouldUseChannel25VideoMapping(info *relaycommon.RelayInfo) bool {
	if info == nil || info.Action == constant.TaskActionRemix {
		return false
	}
	requestPath := strings.Split(strings.TrimSpace(info.RequestURLPath), "?")[0]
	if !strings.HasPrefix(requestPath, "/v1/videos") {
		return false
	}
	base := strings.ToLower(strings.TrimSpace(info.ChannelBaseUrl))
	if info.ChannelId != 25 && !strings.Contains(base, "xn--1ys141f4ks.com") {
		return false
	}
	return isChannel25VideoModel(strings.ToLower(strings.TrimSpace(info.UpstreamModelName)))
}

func isChannel25VideoModel(modelName string) bool {
	if modelName == "sora-2" {
		return true
	}
	return strings.HasPrefix(modelName, "veo3.1")
}

func channel25VideoModelForRequest(info *relaycommon.RelayInfo, bodyMap map[string]interface{}) string {
	modelName := ""
	if info != nil {
		modelName = strings.TrimSpace(info.UpstreamModelName)
	}
	if modelName == "" && bodyMap != nil {
		modelName, _ = firstStringLike(bodyMap["model"])
	}
	if bodyMap == nil {
		return modelName
	}
	resolution, _ := firstStringLike(bodyMap["resolution_name"])
	return channel25VideoModelForResolution(modelName, resolution)
}

func channel25VideoModelForResolution(modelName string, resolution string) string {
	modelName = strings.TrimSpace(modelName)
	if !strings.HasPrefix(strings.ToLower(modelName), "veo3.1") {
		return modelName
	}
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	if resolution == "720p" || resolution == "hd" {
		if !strings.HasSuffix(strings.ToLower(modelName), "-720p") {
			return modelName + "-720p"
		}
		return modelName
	}
	if resolution == "1080p" || resolution == "fullhd" || resolution == "full_hd" || resolution == "fhd" {
		return strings.TrimSuffix(modelName, "-720p")
	}
	return modelName
}

func mapChannel25VideoJSONBody(bodyMap map[string]interface{}) {
	if bodyMap == nil {
		return
	}
	if _, ok := bodyMap["aspect_ratio"]; !ok {
		if ratio, ok := firstStringLike(bodyMap["ratio"]); ok && ratio != "" {
			bodyMap["aspect_ratio"] = ratio
		} else if size, ok := firstStringLike(bodyMap["size"]); ok {
			if ratio := videoRatioFromSize(size); ratio != "" {
				bodyMap["aspect_ratio"] = ratio
			}
		}
	}
	mergeChannel25FrameFields(bodyMap)
	if _, ok := bodyMap["type"]; !ok {
		if videoType, ok := firstStringLike(bodyMap["video_type"]); ok && videoType != "" {
			bodyMap["type"] = videoType
		} else {
			bodyMap["type"] = channel25VideoType(bodyMap)
		}
	}
	delete(bodyMap, "ratio")
	delete(bodyMap, "resolution_name")
	delete(bodyMap, "preset")
	delete(bodyMap, "video_type")
	delete(bodyMap, "reference_mode")
}

func channel25VideoType(bodyMap map[string]interface{}) int {
	mode, _ := firstStringLike(bodyMap["reference_mode"])
	mode = strings.ToLower(strings.TrimSpace(mode))
	modelName, _ := firstStringLike(bodyMap["model"])
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	if mode == "components" || mode == "reference" || strings.Contains(modelName, "components") {
		return 3
	}
	if channel25HasReferenceImage(bodyMap) {
		return 2
	}
	return 1
}

func channel25HasReferenceImage(bodyMap map[string]interface{}) bool {
	for _, key := range []string{"image", "images", "input_reference", "input_reference[]", "first_frame", "last_frame", "reference_images", "reference_images[]"} {
		if v, ok := bodyMap[key]; ok && hasNonEmptyValue(v) {
			return true
		}
	}
	return false
}

func hasNonEmptyValue(v interface{}) bool {
	switch value := v.(type) {
	case string:
		return strings.TrimSpace(value) != ""
	case []string:
		return len(value) > 0
	case []interface{}:
		return len(value) > 0
	default:
		return v != nil
	}
}

func mergeChannel25FrameFields(bodyMap map[string]interface{}) {
	if bodyMap == nil {
		return
	}
	var images []string
	for _, key := range []string{"first_frame", "last_frame", "reference_images", "reference_images[]", "input_reference", "input_reference[]"} {
		if v, ok := bodyMap[key]; ok {
			images = appendStringValues(images, v)
		}
	}
	if len(images) > 0 {
		if _, hasImage := bodyMap["image"]; !hasImage {
			bodyMap["image"] = images[0]
		}
		if _, hasImages := bodyMap["images"]; !hasImages {
			bodyMap["images"] = images
		}
	}
	delete(bodyMap, "first_frame")
	delete(bodyMap, "last_frame")
	delete(bodyMap, "reference_images")
	delete(bodyMap, "reference_images[]")
	delete(bodyMap, "input_reference")
	delete(bodyMap, "input_reference[]")
}

func appendStringValues(dst []string, v interface{}) []string {
	switch value := v.(type) {
	case string:
		if strings.TrimSpace(value) != "" {
			dst = append(dst, value)
		}
	case []string:
		for _, item := range value {
			if strings.TrimSpace(item) != "" {
				dst = append(dst, item)
			}
		}
	case []interface{}:
		for _, item := range value {
			if text, ok := firstStringLike(item); ok && strings.TrimSpace(text) != "" {
				dst = append(dst, text)
			}
		}
	}
	return dst
}

func addChannel25VideoReferenceFiles(bodyMap map[string]interface{}, files map[string][]*multipart.FileHeader) error {
	if bodyMap == nil || len(files) == 0 {
		return nil
	}
	var images []string
	for _, fieldName := range []string{"input_reference[]", "input_reference", "image", "images", "images[]", "first_frame", "last_frame", "reference_images[]", "reference_images"} {
		for _, fh := range files[fieldName] {
			dataURL, err := multipartFileHeaderToDataURL(fh)
			if err != nil {
				return err
			}
			if dataURL != "" {
				images = append(images, dataURL)
			}
		}
	}
	if len(images) == 0 {
		return nil
	}
	bodyMap["image"] = images[0]
	bodyMap["images"] = images
	return nil
}

func buildChannel25VideoMappedBodyForBilling(c *gin.Context, info *relaycommon.RelayInfo) (map[string]interface{}, bool, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, false, err
	}
	cachedBody, err := storage.Bytes()
	if err != nil {
		return nil, false, err
	}
	contentType := c.GetHeader("Content-Type")
	if strings.HasPrefix(contentType, "application/json") {
		var bodyMap map[string]interface{}
		if err := common.Unmarshal(cachedBody, &bodyMap); err != nil {
			return nil, false, err
		}
		bodyMap["model"] = channel25VideoModelForRequest(info, bodyMap)
		mapChannel25VideoJSONBody(bodyMap)
		return bodyMap, true, nil
	}
	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		values, err := parseURLEncodedForm(cachedBody)
		if err != nil {
			return nil, false, err
		}
		bodyMap := formValuesToMap(values)
		bodyMap["model"] = channel25VideoModelForRequest(info, bodyMap)
		mapChannel25VideoJSONBody(bodyMap)
		return bodyMap, true, nil
	}
	if strings.HasPrefix(contentType, "multipart/form-data") {
		formData, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return nil, false, err
		}
		bodyMap := formValuesToMap(formData.Value)
		bodyMap["model"] = channel25VideoModelForRequest(info, bodyMap)
		if err := addChannel25VideoReferenceFiles(bodyMap, formData.File); err != nil {
			return nil, false, err
		}
		mapChannel25VideoJSONBody(bodyMap)
		return bodyMap, true, nil
	}
	return nil, false, nil
}

func mapGPT2APIVideoJSONBody(bodyMap map[string]interface{}) {
	if bodyMap == nil {
		return
	}
	if _, ok := bodyMap["duration"]; !ok {
		if seconds, ok := firstStringLike(bodyMap["seconds"]); ok && seconds != "" {
			if secondsInt, err := strconv.Atoi(seconds); err == nil {
				bodyMap["duration"] = secondsInt
			} else {
				bodyMap["duration"] = seconds
			}
		}
	}
	if _, ok := bodyMap["ratio"]; !ok {
		if size, ok := firstStringLike(bodyMap["size"]); ok {
			if ratio := videoRatioFromSize(size); ratio != "" {
				bodyMap["ratio"] = ratio
			}
		}
	}
	if _, ok := bodyMap["quality"]; !ok {
		if resolution, ok := firstStringLike(bodyMap["resolution_name"]); ok {
			if quality := videoQualityFromResolutionName(resolution); quality != "" {
				bodyMap["quality"] = quality
			}
		}
	}
	if _, ok := bodyMap["image"]; !ok {
		if inputReference, ok := firstStringLike(bodyMap["input_reference"]); ok && inputReference != "" {
			bodyMap["image"] = inputReference
		}
	}
	bodyMap["async"] = true
	delete(bodyMap, "seconds")
	delete(bodyMap, "size")
	delete(bodyMap, "resolution_name")
	delete(bodyMap, "preset")
	delete(bodyMap, "input_reference")
}

func writeGPT2APIVideoMappedFields(writer *multipart.Writer, values map[string][]string) {
	written := map[string]bool{"model": true}
	for key, vals := range values {
		if key == "model" || key == "seconds" || key == "size" || key == "resolution_name" || key == "preset" {
			continue
		}
		for _, v := range vals {
			writer.WriteField(key, v)
			written[key] = true
		}
	}
	if !written["duration"] {
		if seconds := firstFormValue(values, "seconds"); seconds != "" {
			writer.WriteField("duration", seconds)
		}
	}
	if !written["ratio"] {
		if ratio := videoRatioFromSize(firstFormValue(values, "size")); ratio != "" {
			writer.WriteField("ratio", ratio)
		}
	}
	if !written["quality"] {
		if quality := videoQualityFromResolutionName(firstFormValue(values, "resolution_name")); quality != "" {
			writer.WriteField("quality", quality)
		}
	}
	writer.WriteField("async", "true")
}

func formValuesToMap(values map[string][]string) map[string]interface{} {
	bodyMap := make(map[string]interface{}, len(values))
	for key, vals := range values {
		if len(vals) == 0 {
			continue
		}
		if len(vals) == 1 {
			bodyMap[key] = vals[0]
			continue
		}
		items := make([]string, 0, len(vals))
		for _, v := range vals {
			items = append(items, v)
		}
		bodyMap[key] = items
	}
	return bodyMap
}

func addGPT2APIVideoInputReferenceFiles(bodyMap map[string]interface{}, files map[string][]*multipart.FileHeader) error {
	if bodyMap == nil || len(files) == 0 {
		return nil
	}
	var images []string
	for _, fieldName := range []string{"input_reference[]", "input_reference", "image"} {
		for _, fh := range files[fieldName] {
			dataURL, err := multipartFileHeaderToDataURL(fh)
			if err != nil {
				return err
			}
			if dataURL != "" {
				images = append(images, dataURL)
			}
		}
	}
	if len(images) == 0 {
		return nil
	}
	bodyMap["image"] = images[0]
	if len(images) > 1 {
		bodyMap["images"] = images
	}
	return nil
}

func multipartFileHeaderToDataURL(fh *multipart.FileHeader) (string, error) {
	if fh == nil {
		return "", nil
	}
	f, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", nil
	}
	ct := strings.TrimSpace(fh.Header.Get("Content-Type"))
	if ct == "" || ct == "application/octet-stream" {
		ct = http.DetectContentType(data)
	}
	if ct == "" {
		ct = "application/octet-stream"
	}
	return "data:" + ct + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func parseURLEncodedForm(body []byte) (map[string][]string, error) {
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	return map[string][]string(values), nil
}

func multipartFormHasFiles(files map[string][]*multipart.FileHeader) bool {
	for _, headers := range files {
		if len(headers) > 0 {
			return true
		}
	}
	return false
}

func firstFormValue(values map[string][]string, key string) string {
	if values == nil || len(values[key]) == 0 {
		return ""
	}
	return values[key][0]
}

func firstStringLike(v interface{}) (string, bool) {
	switch value := v.(type) {
	case string:
		return value, true
	case float64:
		return strconv.Itoa(int(value)), true
	case int:
		return strconv.Itoa(value), true
	case int64:
		return strconv.FormatInt(value, 10), true
	default:
		return "", false
	}
}

func videoRatioFromSize(size string) string {
	size = strings.TrimSpace(strings.ToLower(size))
	if size == "" || size == "auto" {
		return ""
	}
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return ""
	}
	w, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return ""
	}
	g := gcd(w, h)
	return fmt.Sprintf("%d:%d", w/g, h/g)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

func videoQualityFromResolutionName(resolution string) string {
	switch strings.ToLower(strings.TrimSpace(resolution)) {
	case "1080p", "fullhd", "full_hd", "fhd":
		return "fullhd"
	case "720p", "hd":
		return "hd"
	case "480p", "sd":
		return "sd"
	default:
		return ""
	}
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Parse Sora response
	var dResp responseTask
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	upstreamID := dResp.ID
	if upstreamID == "" {
		upstreamID = dResp.TaskID
	}
	if upstreamID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	// 使用公开 task_xxxx ID 返回给客户端
	dResp.ID = info.PublicTaskID
	dResp.TaskID = info.PublicTaskID
	c.JSON(http.StatusOK, dResp)
	return upstreamID, responseBody, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	baseUrl = strings.TrimRight(baseUrl, "/")
	taskID, ok := body["task_id"].(string)
	if !ok || taskID == "" {
		return nil, fmt.Errorf("invalid task_id")
	}

	uriPath := fmt.Sprintf("/v1/videos/%s", taskID)
	if rawPath, _ := body["request_path"].(string); strings.HasPrefix(rawPath, "/v1/images/generations") || strings.HasPrefix(rawPath, "/v1/images/edits") || strings.HasPrefix(rawPath, "/v1/images/edit") {
		uriPath = path.Join("/v1/images/generations", taskID)
	} else if strings.HasPrefix(rawPath, "/v1/video/generations") || shouldUseGPT2APIVideoGenerationsFetch(baseUrl, body, rawPath) {
		uriPath = path.Join("/v1/video/generations", taskID)
	}
	uri := baseUrl + uriPath

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

func shouldUseGPT2APIVideoGenerationsFetch(baseURL string, body map[string]any, rawPath string) bool {
	if !strings.HasPrefix(rawPath, "/v1/videos") {
		return false
	}
	base := strings.ToLower(strings.TrimSpace(baseURL))
	modelName := ""
	if body != nil {
		if v, ok := body["model"].(string); ok {
			modelName = strings.ToLower(strings.TrimSpace(v))
		} else if v, ok := body["upstream_model_name"].(string); ok {
			modelName = strings.ToLower(strings.TrimSpace(v))
		}
	}
	return strings.Contains(base, "gpt2api.com") && isGPT2APIVideoGenerationsModel(modelName)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code:   0,
		TaskID: firstNonEmpty(resTask.TaskID, resTask.ID),
	}
	if imageURL := firstImageArtifactURL(respBody); imageURL != "" {
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = imageURL
		return &taskResult, nil
	}

	switch resTask.Status {
	case "queued", "pending":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "0%"
	case "processing", "in_progress", "running":
		taskResult.Status = model.TaskStatusInProgress
	case "completed", "succeeded", "success":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		// Url intentionally left empty — the caller constructs the proxy URL using the public task ID
	case "failed", "cancelled":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		if resTask.Error != nil {
			taskResult.Reason = resTask.Error.Message
		} else {
			taskResult.Reason = "task failed"
		}
	default:
	}
	if resTask.Progress >= 0 && resTask.Progress <= 100 {
		if resTask.Progress > 0 || taskResult.Status != "" {
			taskResult.Progress = fmt.Sprintf("%d%%", resTask.Progress)
		}
	}
	if taskResult.Status == model.TaskStatusSuccess && taskResult.Progress == "0%" {
		taskResult.Progress = "100%"
	}

	return &taskResult, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

type imageArtifact struct {
	Url     string `json:"url"`
	B64Json string `json:"b64_json"`
}

func firstImageArtifactURL(respBody []byte) string {
	var raw struct {
		Data   []imageArtifact `json:"data"`
		Result struct {
			Data []imageArtifact `json:"data"`
		} `json:"result"`
	}
	if err := common.Unmarshal(respBody, &raw); err != nil {
		return ""
	}
	if url := firstImageArtifactURLFromItems(raw.Data); url != "" {
		return url
	}
	return firstImageArtifactURLFromItems(raw.Result.Data)
}

func firstImageArtifactURLFromItems(items []imageArtifact) string {
	for _, item := range items {
		if item.Url != "" {
			return item.Url
		}
		if item.B64Json != "" {
			return "data:image/png;base64," + item.B64Json
		}
	}
	return ""
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	data := task.Data
	if isOpenAIImageTaskRequestPath(task.PrivateData.RequestPath) {
		converted, err := convertImageGenerationTaskResponse(data)
		if err != nil {
			return nil, errors.Wrap(err, "convert image task response failed")
		}
		data = converted
	} else if isOpenAIVideoTaskRequestPath(task.PrivateData.RequestPath) {
		converted, err := convertVideoTaskResponseForOpenAIClient(data)
		if err != nil {
			return nil, errors.Wrap(err, "convert video task response failed")
		}
		data = converted
	}
	if strings.EqualFold(strings.TrimSpace(task.PrivateData.ResponseFormat), "b64_json") {
		converted, _, err := imageutil.ConvertImageURLResponseToB64(context.Background(), data)
		if err != nil {
			return nil, errors.Wrap(err, "convert image url response to b64 failed")
		}
		data = converted
	}
	var err error
	if data, err = sjson.SetBytes(data, "id", task.TaskID); err != nil {
		return nil, errors.Wrap(err, "set id failed")
	}
	if data, err = sjson.SetBytes(data, "task_id", task.TaskID); err != nil {
		return nil, errors.Wrap(err, "set task_id failed")
	}
	return data, nil
}

func isOpenAIImageTaskRequestPath(requestPath string) bool {
	path := strings.TrimSpace(requestPath)
	return strings.HasPrefix(path, "/v1/images/generations") || strings.HasPrefix(path, "/v1/images/edits") || strings.HasPrefix(path, "/v1/images/edit")
}

func isOpenAIVideoTaskRequestPath(requestPath string) bool {
	path := strings.TrimSpace(requestPath)
	return strings.HasPrefix(path, "/v1/videos") || strings.HasPrefix(path, "/v1/video/generations")
}

func convertVideoTaskResponseForOpenAIClient(data []byte) ([]byte, error) {
	var payload map[string]any
	if err := common.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	delete(payload, "usage")
	if result, ok := payload["result"].(map[string]any); ok {
		delete(result, "usage")
	}
	if status, ok := payload["status"].(string); ok {
		payload["status"] = mapVideoTaskStatusForOpenAIClient(status)
	}
	return common.Marshal(payload)
}

func mapVideoTaskStatusForOpenAIClient(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "success", "completed":
		return "completed"
	case "processing", "in_progress", "running":
		return "running"
	case "queued", "pending", "submitted":
		return "queued"
	case "failed", "failure":
		return "failed"
	case "cancelled", "canceled":
		return "cancelled"
	default:
		return status
	}
}

func convertImageGenerationTaskResponse(data []byte) ([]byte, error) {
	var payload map[string]any
	if err := common.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	delete(payload, "usage")
	if result, ok := payload["result"].(map[string]any); ok {
		delete(result, "usage")
	}
	if _, hasData := payload["data"]; !hasData {
		if result, ok := payload["result"].(map[string]any); ok {
			if resultData, ok := result["data"]; ok {
				payload["data"] = resultData
			}
		}
	}
	if result, ok := payload["result"].(map[string]any); ok {
		if _, hasResultData := result["data"]; !hasResultData {
			if topData, ok := payload["data"]; ok {
				result["data"] = topData
			}
		}
	} else if topData, ok := payload["data"]; ok {
		payload["result"] = map[string]any{"data": topData}
	}
	return common.Marshal(payload)
}
