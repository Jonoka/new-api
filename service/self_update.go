package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	defaultSelfUpdateRepo     = "moeacgx/new-api"
	selfUpdateUserAgent       = "new-api-self-updater"
	defaultSelfUpdateMaxMB    = 256
	defaultSelfUpdateExitSecs = 2
)

var selfUpdateInProgress atomic.Bool

type GitHubReleaseAsset struct {
	URL                string `json:"url"`
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}

type GitHubRelease struct {
	TagName     string               `json:"tag_name"`
	Name        string               `json:"name"`
	Body        string               `json:"body"`
	HTMLURL     string               `json:"html_url"`
	PublishedAt string               `json:"published_at"`
	Assets      []GitHubReleaseAsset `json:"assets"`
}

type SelfUpdateReleaseInfo struct {
	CurrentVersion           string `json:"current_version"`
	TagName                  string `json:"tag_name"`
	Name                     string `json:"name"`
	Body                     string `json:"body"`
	HTMLURL                  string `json:"html_url"`
	PublishedAt              string `json:"published_at"`
	AssetName                string `json:"asset_name,omitempty"`
	ChecksumAssetName        string `json:"checksum_asset_name,omitempty"`
	UpdateAvailable          bool   `json:"update_available"`
	SelfUpdateSupported      bool   `json:"self_update_supported"`
	SelfUpdateDisabledReason string `json:"self_update_disabled_reason,omitempty"`
}

type SelfUpdateResult struct {
	CurrentVersion    string `json:"current_version"`
	TargetVersion     string `json:"target_version"`
	AssetName         string `json:"asset_name,omitempty"`
	ExecutablePath    string `json:"executable_path,omitempty"`
	RestartScheduled  bool   `json:"restart_scheduled"`
	ExitDelaySeconds  int    `json:"exit_delay_seconds,omitempty"`
	Message           string `json:"message"`
	AlreadyUpToDate   bool   `json:"already_up_to_date"`
	DownloadedBytes   int64  `json:"downloaded_bytes,omitempty"`
	DownloadedSHA256  string `json:"downloaded_sha256,omitempty"`
	ExpectedSHA256    string `json:"expected_sha256,omitempty"`
	RunningInDocker   bool   `json:"running_in_docker"`
	AllowNonDockerRun bool   `json:"allow_non_docker_run"`
}

func GetLatestSelfUpdateRelease(ctx context.Context) (*SelfUpdateReleaseInfo, error) {
	repo := selfUpdateRepo()
	if err := validateSelfUpdateRepo(repo); err != nil {
		return nil, err
	}

	release, err := fetchGitHubRelease(ctx, repo, "latest")
	if err != nil {
		return nil, err
	}

	info := &SelfUpdateReleaseInfo{
		CurrentVersion:      common.Version,
		TagName:             release.TagName,
		Name:                release.Name,
		Body:                release.Body,
		HTMLURL:             release.HTMLURL,
		PublishedAt:         release.PublishedAt,
		UpdateAvailable:     release.TagName != "" && release.TagName != common.Version,
		SelfUpdateSupported: true,
	}

	assetName, checksumName, err := selfUpdateAssetNames(runtime.GOOS, runtime.GOARCH, release.TagName)
	if err != nil {
		info.SelfUpdateSupported = false
		info.SelfUpdateDisabledReason = err.Error()
		return info, nil
	}
	info.AssetName = assetName
	info.ChecksumAssetName = checksumName

	if reason := selfUpdateDisabledReason(runtime.GOOS); reason != "" {
		info.SelfUpdateSupported = false
		info.SelfUpdateDisabledReason = reason
		return info, nil
	}

	if _, ok := findReleaseAsset(release.Assets, assetName); !ok {
		info.SelfUpdateSupported = false
		info.SelfUpdateDisabledReason = fmt.Sprintf("未找到当前平台的更新资产：%s", assetName)
	}

	return info, nil
}

func RunSelfUpdate(ctx context.Context, targetTag string) (*SelfUpdateResult, error) {
	if !selfUpdateInProgress.CompareAndSwap(false, true) {
		return nil, errors.New("已有更新任务正在执行")
	}

	scheduledExit := false
	defer func() {
		if !scheduledExit {
			selfUpdateInProgress.Store(false)
		}
	}()

	repo := selfUpdateRepo()
	if err := validateSelfUpdateRepo(repo); err != nil {
		return nil, err
	}
	if reason := selfUpdateDisabledReason(runtime.GOOS); reason != "" {
		return nil, errors.New(reason)
	}

	releaseRef := "latest"
	if strings.TrimSpace(targetTag) != "" {
		releaseRef = strings.TrimSpace(targetTag)
	}
	release, err := fetchGitHubRelease(ctx, repo, releaseRef)
	if err != nil {
		return nil, err
	}
	if release.TagName == "" {
		return nil, errors.New("GitHub release 缺少 tag_name")
	}

	result := &SelfUpdateResult{
		CurrentVersion:    common.Version,
		TargetVersion:     release.TagName,
		RunningInDocker:   isRunningInDocker(),
		AllowNonDockerRun: common.GetEnvOrDefaultBool("SELF_UPDATE_ALLOW_NON_DOCKER", false),
	}
	if release.TagName == common.Version {
		result.AlreadyUpToDate = true
		result.Message = fmt.Sprintf("已是最新版本：%s", release.TagName)
		return result, nil
	}

	assetName, checksumName, err := selfUpdateAssetNames(runtime.GOOS, runtime.GOARCH, release.TagName)
	if err != nil {
		return nil, err
	}
	result.AssetName = assetName

	asset, ok := findReleaseAsset(release.Assets, assetName)
	if !ok {
		return nil, fmt.Errorf("未找到当前平台的更新资产：%s", assetName)
	}
	assetDownloadURL := releaseAssetDownloadURL(asset)
	if err := validateGitHubDownloadURL(repo, assetDownloadURL); err != nil {
		return nil, err
	}

	expectedSHA256 := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(asset.Digest)), "sha256:")
	if checksumAsset, ok := findReleaseAsset(release.Assets, checksumName); ok {
		checksumDownloadURL := releaseAssetDownloadURL(checksumAsset)
		if err := validateGitHubDownloadURL(repo, checksumDownloadURL); err != nil {
			return nil, err
		}
		manifest, err := downloadString(ctx, checksumDownloadURL, 1<<20)
		if err != nil {
			return nil, err
		}
		expectedSHA256, err = checksumForAsset(manifest, assetName)
		if err != nil {
			return nil, err
		}
	}
	if !isSHA256Hex(expectedSHA256) {
		return nil, fmt.Errorf("未找到 %s 的有效 sha256 校验值", assetName)
	}
	result.ExpectedSHA256 = expectedSHA256

	executablePath, err := currentExecutablePath()
	if err != nil {
		return nil, err
	}
	result.ExecutablePath = executablePath

	tempPath, downloadedBytes, downloadedSHA256, err := downloadAssetToTemp(ctx, assetDownloadURL, filepath.Dir(executablePath), selfUpdateMaxBytes())
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempPath)
	result.DownloadedBytes = downloadedBytes
	result.DownloadedSHA256 = downloadedSHA256

	if !strings.EqualFold(downloadedSHA256, expectedSHA256) {
		return nil, fmt.Errorf("sha256 校验失败：下载值 %s，期望值 %s", downloadedSHA256, expectedSHA256)
	}

	if err := chmodLikeExecutable(tempPath, executablePath); err != nil {
		return nil, err
	}
	if err := os.Rename(tempPath, executablePath); err != nil {
		return nil, fmt.Errorf("替换运行二进制失败：%w", err)
	}

	delay := selfUpdateExitDelay()
	result.RestartScheduled = true
	result.ExitDelaySeconds = int(delay.Seconds())
	result.Message = fmt.Sprintf("已安装 %s，服务将在 %d 秒后退出并等待 Docker 自动重启", release.TagName, result.ExitDelaySeconds)

	scheduledExit = true
	go func(version string, wait time.Duration) {
		time.Sleep(wait)
		common.SysLog(fmt.Sprintf("self update installed %s, exiting for Docker restart", version))
		os.Exit(0)
	}(release.TagName, delay)

	return result, nil
}

func selfUpdateRepo() string {
	repo := strings.TrimSpace(os.Getenv("SELF_UPDATE_REPO"))
	if repo == "" {
		repo = strings.TrimSpace(os.Getenv("SELF_UPDATE_GITHUB_REPO"))
	}
	if repo == "" {
		repo = defaultSelfUpdateRepo
	}
	return repo
}

func validateSelfUpdateRepo(repo string) error {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("SELF_UPDATE_REPO 必须是 owner/repo 格式，当前值：%s", repo)
	}
	for _, part := range parts {
		for _, r := range part {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
				continue
			}
			return fmt.Errorf("SELF_UPDATE_REPO 包含非法字符：%s", repo)
		}
	}
	return nil
}

func fetchGitHubRelease(ctx context.Context, repo string, ref string) (*GitHubRelease, error) {
	var endpoint string
	if ref == "latest" || strings.TrimSpace(ref) == "" {
		endpoint = fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	} else {
		endpoint = fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, url.PathEscape(ref))
	}

	var release GitHubRelease
	if err := doGitHubJSON(ctx, endpoint, &release); err != nil {
		return nil, err
	}
	return &release, nil
}

func doGitHubJSON(ctx context.Context, endpoint string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	setGitHubHeaders(req)

	resp, err := selfUpdateHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("GitHub API 请求失败：%s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return common.DecodeJson(resp.Body, dest)
}

func downloadString(ctx context.Context, downloadURL string, maxBytes int64) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", err
	}
	setGitHubHeaders(req)

	resp, err := selfUpdateHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("下载校验文件失败：%s %s", resp.Status, strings.TrimSpace(string(body)))
	}

	reader := io.LimitReader(resp.Body, maxBytes+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	if int64(len(data)) > maxBytes {
		return "", errors.New("校验文件过大")
	}
	return string(data), nil
}

func downloadAssetToTemp(ctx context.Context, downloadURL string, dir string, maxBytes int64) (string, int64, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", 0, "", err
	}
	setGitHubHeaders(req)

	resp, err := selfUpdateHTTPClient().Do(req)
	if err != nil {
		return "", 0, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", 0, "", fmt.Errorf("下载更新资产失败：%s %s", resp.Status, strings.TrimSpace(string(body)))
	}

	tempFile, err := os.CreateTemp(dir, ".new-api-update-*")
	if err != nil {
		return "", 0, "", err
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		_ = tempFile.Close()
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	hash := sha256.New()
	limited := &io.LimitedReader{R: resp.Body, N: maxBytes + 1}
	written, err := io.Copy(io.MultiWriter(tempFile, hash), limited)
	if err != nil {
		return "", 0, "", err
	}
	if written > maxBytes {
		return "", 0, "", fmt.Errorf("更新资产超过限制：%d bytes", maxBytes)
	}
	if err := tempFile.Sync(); err != nil {
		return "", 0, "", err
	}
	if err := tempFile.Close(); err != nil {
		return "", 0, "", err
	}

	cleanup = false
	return tempPath, written, fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func setGitHubHeaders(req *http.Request) {
	if req.URL != nil && req.URL.Host == "api.github.com" && strings.Contains(req.URL.Path, "/releases/assets/") {
		req.Header.Set("Accept", "application/octet-stream")
	} else {
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	req.Header.Set("User-Agent", selfUpdateUserAgent)
	if token := strings.TrimSpace(os.Getenv("SELF_UPDATE_GITHUB_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func selfUpdateHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy:             http.ProxyFromEnvironment,
		ForceAttemptHTTP2: true,
	}
	if common.TLSInsecureSkipVerify {
		transport.TLSClientConfig = common.InsecureTLSConfig
	}
	return &http.Client{
		Timeout:   10 * time.Minute,
		Transport: transport,
	}
}

func selfUpdateAssetNames(goos string, goarch string, tagName string) (assetName string, checksumName string, err error) {
	if goos != "linux" {
		return "", "", fmt.Errorf("Docker 容器内自更新仅支持 Linux，当前系统：%s/%s", goos, goarch)
	}
	switch goarch {
	case "amd64":
		return "new-api-" + tagName, "checksums-linux.txt", nil
	case "arm64":
		return "new-api-arm64-" + tagName, "checksums-linux.txt", nil
	default:
		return "", "", fmt.Errorf("暂不支持当前架构：%s/%s", goos, goarch)
	}
}

func checksumForAsset(manifest string, assetName string) (string, error) {
	for _, line := range strings.Split(manifest, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name != assetName {
			continue
		}
		checksum := strings.ToLower(fields[0])
		if !isSHA256Hex(checksum) {
			return "", fmt.Errorf("%s 的 sha256 格式无效", assetName)
		}
		return checksum, nil
	}
	return "", fmt.Errorf("校验文件中未找到资产：%s", assetName)
}

func isSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return false
	}
	return true
}

func findReleaseAsset(assets []GitHubReleaseAsset, name string) (GitHubReleaseAsset, bool) {
	for _, asset := range assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return GitHubReleaseAsset{}, false
}

func releaseAssetDownloadURL(asset GitHubReleaseAsset) string {
	if strings.TrimSpace(asset.URL) != "" {
		return strings.TrimSpace(asset.URL)
	}
	return strings.TrimSpace(asset.BrowserDownloadURL)
}

func validateGitHubDownloadURL(repo string, downloadURL string) error {
	parsed, err := url.Parse(downloadURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("更新资产必须使用 https 下载：%s", downloadURL)
	}
	if parsed.Host != "github.com" && parsed.Host != "api.github.com" && parsed.Host != "objects.githubusercontent.com" {
		return fmt.Errorf("更新资产下载域名不受信任：%s", parsed.Host)
	}
	if parsed.Host == "github.com" {
		expectedPrefix := "/" + repo + "/releases/download/"
		if !strings.HasPrefix(parsed.Path, expectedPrefix) {
			return fmt.Errorf("更新资产路径不属于仓库 %s：%s", repo, parsed.Path)
		}
	}
	if parsed.Host == "api.github.com" {
		expectedPrefix := "/repos/" + repo + "/releases/assets/"
		if !strings.HasPrefix(parsed.Path, expectedPrefix) {
			return fmt.Errorf("更新资产 API 路径不属于仓库 %s：%s", repo, parsed.Path)
		}
	}
	return nil
}

func selfUpdateDisabledReason(goos string) string {
	if goos != "linux" {
		return fmt.Sprintf("Docker 容器内自更新仅支持 Linux，当前系统：%s/%s", goos, runtime.GOARCH)
	}
	if isRunningInDocker() || common.GetEnvOrDefaultBool("SELF_UPDATE_ALLOW_NON_DOCKER", false) {
		return ""
	}
	return "当前进程未检测到 Docker 容器环境；如确认需要启用，请设置 SELF_UPDATE_ALLOW_NON_DOCKER=true"
}

func isRunningInDocker() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	for _, file := range []string{"/proc/1/cgroup", "/proc/self/cgroup"} {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		text := string(data)
		if strings.Contains(text, "docker") ||
			strings.Contains(text, "containerd") ||
			strings.Contains(text, "kubepods") ||
			strings.Contains(text, "libpod") ||
			strings.Contains(text, "podman") {
			return true
		}
	}
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" || os.Getenv("container") != "" {
		return true
	}
	return false
}

func currentExecutablePath() (string, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(executablePath); err == nil {
		executablePath = resolved
	}
	return executablePath, nil
}

func chmodLikeExecutable(tempPath string, executablePath string) error {
	mode := os.FileMode(0755)
	if info, err := os.Stat(executablePath); err == nil {
		mode = info.Mode().Perm()
	}
	return os.Chmod(tempPath, mode)
}

func selfUpdateMaxBytes() int64 {
	maxMB := defaultSelfUpdateMaxMB
	if raw := strings.TrimSpace(os.Getenv("SELF_UPDATE_MAX_MB")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			maxMB = parsed
		}
	}
	return int64(maxMB) << 20
}

func selfUpdateExitDelay() time.Duration {
	seconds := defaultSelfUpdateExitSecs
	if raw := strings.TrimSpace(os.Getenv("SELF_UPDATE_EXIT_DELAY_SECONDS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			seconds = parsed
		}
	}
	return time.Duration(seconds) * time.Second
}
