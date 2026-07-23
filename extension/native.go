package extension

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var ErrNativeAssetNotFound = errors.New("native extension asset not found")

type NativeAsset struct {
	File        *os.File
	ContentType string
	Size        int64
}

// nativeAssetRevision 校验全部原生界面资源，并计算与声明顺序和文件内容绑定的修订号。
func nativeAssetRevision(moduleDir string, manifest Manifest) (string, error) {
	hasher := sha256.New()
	hasNativePage := false
	module := Module{Manifest: manifest, Path: moduleDir}

	for _, page := range manifest.UI.Pages {
		if page.Render == nil || page.Render.Type != RenderTypeNative {
			continue
		}
		hasNativePage = true
		if err := hashNativeRenderTarget(hasher, module, page.Key, "default", page.Render.Targets.Default); err != nil {
			return "", err
		}
		if err := hashNativeRenderTarget(hasher, module, page.Key, "classic", page.Render.Targets.Classic); err != nil {
			return "", err
		}
	}

	if !hasNativePage {
		return "", nil
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func hashNativeRenderTarget(hasher hash.Hash, module Module, pageKey, targetName string, target NativeRenderTarget) error {
	if err := hashNativeAsset(hasher, module, pageKey, targetName, "entry", target.Entry); err != nil {
		return err
	}
	for index, style := range target.Styles {
		if err := hashNativeAsset(hasher, module, pageKey, targetName, fmt.Sprintf("style-%d", index), style); err != nil {
			return err
		}
	}
	return nil
}

func hashNativeAsset(hasher hash.Hash, module Module, pageKey, targetName, assetName, assetPath string) error {
	if err := writeNativeRevisionValue(hasher, pageKey); err != nil {
		return err
	}
	if err := writeNativeRevisionValue(hasher, targetName); err != nil {
		return err
	}
	if err := writeNativeRevisionValue(hasher, assetName); err != nil {
		return err
	}
	if err := writeNativeRevisionValue(hasher, assetPath); err != nil {
		return err
	}

	file, size, err := openNativeAssetFile(module, assetPath)
	if err != nil {
		return fmt.Errorf("%s target %s asset %s (%s): %w", pageKey, targetName, assetName, assetPath, err)
	}

	var sizeBytes [8]byte
	binary.BigEndian.PutUint64(sizeBytes[:], uint64(size))
	if _, err := hasher.Write(sizeBytes[:]); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := io.Copy(hasher, file); err != nil {
		_ = file.Close()
		return fmt.Errorf("read native asset %s: %w", assetPath, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close native asset %s: %w", assetPath, err)
	}
	return nil
}

func writeNativeRevisionValue(hasher hash.Hash, value string) error {
	var lengthBytes [8]byte
	binary.BigEndian.PutUint64(lengthBytes[:], uint64(len(value)))
	if _, err := hasher.Write(lengthBytes[:]); err != nil {
		return err
	}
	_, err := io.WriteString(hasher, value)
	return err
}

// OpenNativeAsset 只打开 manifest 白名单中的原生界面资源，不提供目录或首页回退。
func (m *Manager) OpenNativeAsset(moduleID, pageKey, targetName, assetName string, role int) (*NativeAsset, error) {
	module, exists := m.Get(strings.TrimSpace(moduleID))
	if !exists {
		return nil, fmt.Errorf("%w: module does not exist", ErrNativeAssetNotFound)
	}
	if module.Error != "" {
		return nil, errors.New("module manifest is invalid")
	}
	if !module.Enabled {
		return nil, errors.New("module is disabled")
	}
	if !roleAllowed(role, module.Permissions.Roles) {
		return nil, errors.New("module permission denied")
	}
	if module.Runtime.Type != RuntimeTypeStatic {
		return nil, errors.New("native extension pages require static runtime")
	}
	if !hasPermissionCapability(module.Permissions.Capabilities, CapabilityUINative) {
		return nil, errors.New("module has not declared ui.native capability")
	}

	page, exists := findNativePage(module.UI.Pages, pageKey)
	if !exists || page.Render == nil || page.Render.Type != RenderTypeNative || page.Render.SDK != NativeSDKV1 {
		return nil, fmt.Errorf("%w: native page does not exist", ErrNativeAssetNotFound)
	}
	target, exists := selectNativeRenderTarget(page.Render.Targets, targetName)
	if !exists {
		return nil, fmt.Errorf("%w: target does not exist", ErrNativeAssetNotFound)
	}
	assetPath, contentType, exists := selectNativeAsset(target, assetName)
	if !exists {
		return nil, fmt.Errorf("%w: asset does not exist", ErrNativeAssetNotFound)
	}
	if err := validateNativeAssetPath(assetPath, nativeAssetExtensions(assetName)...); err != nil {
		return nil, errors.New("native asset path is invalid")
	}

	file, size, err := openNativeAssetFile(module, assetPath)
	if err != nil {
		return nil, err
	}
	return &NativeAsset{
		File:        file,
		ContentType: contentType,
		Size:        size,
	}, nil
}

func findNativePage(pages []Page, pageKey string) (Page, bool) {
	for _, page := range pages {
		if page.Key == pageKey {
			return page, true
		}
	}
	return Page{}, false
}

func selectNativeRenderTarget(targets NativeRenderTargets, name string) (NativeRenderTarget, bool) {
	switch name {
	case "default":
		return targets.Default, true
	case "classic":
		return targets.Classic, true
	default:
		return NativeRenderTarget{}, false
	}
}

func selectNativeAsset(target NativeRenderTarget, name string) (string, string, bool) {
	if name == "entry" {
		return target.Entry, "text/javascript; charset=utf-8", true
	}
	if !strings.HasPrefix(name, "style-") {
		return "", "", false
	}
	indexText := strings.TrimPrefix(name, "style-")
	if indexText == "" {
		return "", "", false
	}
	for _, character := range indexText {
		if character < '0' || character > '9' {
			return "", "", false
		}
	}
	index, err := strconv.Atoi(indexText)
	if err != nil || index < 0 || index >= len(target.Styles) {
		return "", "", false
	}
	return target.Styles[index], "text/css; charset=utf-8", true
}

func nativeAssetExtensions(assetName string) []string {
	if assetName == "entry" {
		return []string{".js", ".mjs"}
	}
	return []string{".css"}
}

func openNativeAssetFile(module Module, assetPath string) (*os.File, int64, error) {
	if strings.TrimSpace(module.Path) == "" {
		return nil, 0, errors.New("module path is unavailable")
	}
	staticDir := strings.TrimSpace(module.Runtime.StaticDir)
	if staticDir == "" {
		staticDir = DefaultStaticDir
	}
	staticDir = filepath.Clean(filepath.FromSlash(staticDir))
	if filepath.IsAbs(staticDir) || staticDir == "." || staticDir == ".." || strings.HasPrefix(staticDir, ".."+string(filepath.Separator)) {
		return nil, 0, errors.New("module static directory is invalid")
	}

	root, err := filepath.Abs(filepath.Join(module.Path, staticDir))
	if err != nil {
		return nil, 0, errors.New("module static directory is unavailable")
	}
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		return nil, 0, errors.New("module static directory is unavailable")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, 0, errors.New("module static directory is unavailable")
	}
	modulePath, err := filepath.Abs(module.Path)
	if err != nil {
		return nil, 0, errors.New("module path is unavailable")
	}
	resolvedModulePath, err := filepath.EvalSymlinks(modulePath)
	if err != nil {
		return nil, 0, errors.New("module path is unavailable")
	}
	if err := ensurePathInside(resolvedModulePath, resolvedRoot); err != nil {
		return nil, 0, errors.New("module static directory escapes module path")
	}

	target := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(assetPath, "/")))
	if err := ensurePathInside(root, target); err != nil {
		return nil, 0, errors.New("native asset path escapes module static directory")
	}
	targetInfo, err := os.Lstat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, fmt.Errorf("%w: file does not exist", ErrNativeAssetNotFound)
		}
		return nil, 0, errors.New("native asset is unavailable")
	}
	if !targetInfo.Mode().IsRegular() {
		return nil, 0, errors.New("native asset must be a regular file")
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return nil, 0, errors.New("native asset is unavailable")
	}
	if err := ensurePathInside(resolvedRoot, resolvedTarget); err != nil {
		return nil, 0, errors.New("native asset path escapes module static directory")
	}

	file, err := os.Open(resolvedTarget)
	if err != nil {
		return nil, 0, errors.New("native asset is unavailable")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, 0, errors.New("native asset must be a regular file")
	}
	return file, info.Size(), nil
}
