package extension

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	MaxInstallArchiveBytes         int64 = 100 << 20
	maxInstallArchiveExpandedBytes int64 = 200 << 20
)

type stateFile struct {
	Modules map[string]moduleState `json:"modules"`
}

type moduleState struct {
	Enabled bool `json:"enabled"`
}

type Manager struct {
	rootDir string
	mu      sync.RWMutex
	modules map[string]Module
	state   stateFile
}

var DefaultManager = NewManager(resolveRootDir())

func NewManager(rootDir string) *Manager {
	if strings.TrimSpace(rootDir) == "" {
		rootDir = DefaultRootDir
	}
	return &Manager{
		rootDir: rootDir,
		modules: map[string]Module{},
		state: stateFile{
			Modules: map[string]moduleState{},
		},
	}
}

func Init() error {
	DefaultManager = NewManager(resolveRootDir())
	return DefaultManager.Scan()
}

func resolveRootDir() string {
	if value := strings.TrimSpace(os.Getenv("EXTENSIONS_ROOT")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("MODULES_ROOT")); value != "" {
		return value
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		legacyDir := filepath.Join("/data", DefaultRootDir)
		if hasInstalledModules(legacyDir) {
			return legacyDir
		}
		return "/data/modules"
	}
	return DefaultRootDir
}

func hasInstalledModules(rootDir string) bool {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if regularFileExists(filepath.Join(rootDir, entry.Name(), "manifest.json")) {
			return true
		}
	}
	return false
}

func (m *Manager) RootDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rootDir
}

func (m *Manager) Scan() error {
	if err := os.MkdirAll(m.rootDir, 0755); err != nil {
		return err
	}

	state, err := m.loadState()
	if err != nil {
		return err
	}

	nextModules := map[string]Module{}
	entries, err := os.ReadDir(m.rootDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		moduleDir := filepath.Join(m.rootDir, entry.Name())
		manifestPath := filepath.Join(moduleDir, "manifest.json")
		manifestBytes, readErr := os.ReadFile(manifestPath)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				continue
			}
			nextModules[entry.Name()] = Module{
				Manifest: Manifest{ID: entry.Name(), Name: entry.Name()},
				Path:     moduleDir,
				Error:    readErr.Error(),
			}
			continue
		}

		var manifest Manifest
		if err := common.Unmarshal(manifestBytes, &manifest); err != nil {
			nextModules[entry.Name()] = Module{
				Manifest: Manifest{ID: entry.Name(), Name: entry.Name()},
				Path:     moduleDir,
				Error:    err.Error(),
			}
			continue
		}
		manifest.ID = strings.TrimSpace(manifest.ID)
		if err := manifest.Validate(); err != nil {
			if manifest.ID == "" {
				manifest.ID = entry.Name()
			}
			if manifest.Name == "" {
				manifest.Name = manifest.ID
			}
			nextModules[manifest.ID] = Module{
				Manifest: manifest,
				Path:     moduleDir,
				Error:    err.Error(),
			}
			continue
		}

		enabled := false
		if saved, ok := state.Modules[manifest.ID]; ok {
			enabled = saved.Enabled
		}
		nextModules[manifest.ID] = Module{
			Manifest: manifest,
			Enabled:  enabled,
			Path:     moduleDir,
		}
	}

	m.mu.Lock()
	m.modules = nextModules
	m.state = state
	m.mu.Unlock()
	return nil
}

func (m *Manager) List(role int, includeDisabled bool) []PublicModule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]PublicModule, 0, len(m.modules))
	for _, module := range m.modules {
		if !includeDisabled && !module.Enabled {
			continue
		}
		if !roleAllowed(role, module.Permissions.Roles) && !includeDisabled {
			continue
		}
		result = append(result, module.Public(includeDisabled))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

func (m *Manager) Get(id string) (Module, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	module, ok := m.modules[id]
	return module, ok
}

func (m *Manager) SetEnabled(id string, enabled bool) (Module, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	module, ok := m.modules[id]
	if !ok {
		return Module{}, errors.New("module not found")
	}
	if module.Error != "" {
		return Module{}, errors.New("module manifest is invalid: " + module.Error)
	}
	module.Enabled = enabled
	m.modules[id] = module
	if m.state.Modules == nil {
		m.state.Modules = map[string]moduleState{}
	}
	m.state.Modules[id] = moduleState{Enabled: enabled}
	if err := m.saveStateLocked(); err != nil {
		return Module{}, err
	}
	return module, nil
}

func (m *Manager) Uninstall(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	module, ok := m.modules[id]
	if !ok {
		return errors.New("module not found")
	}
	targetDir, err := safeModuleTargetDir(m.rootDir, id)
	if err != nil {
		return err
	}
	if module.Path != "" && !sameCleanPath(module.Path, targetDir) {
		return errors.New("module path does not match module id")
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return err
	}
	delete(m.modules, id)
	if m.state.Modules != nil {
		delete(m.state.Modules, id)
	}
	return m.saveStateLocked()
}

func (m *Manager) InstallArchive(readerAt io.ReaderAt, size int64) (Module, error) {
	if readerAt == nil {
		return Module{}, errors.New("module archive is required")
	}
	if size <= 0 {
		return Module{}, errors.New("module archive is empty")
	}
	if size > MaxInstallArchiveBytes {
		return Module{}, fmt.Errorf("module archive exceeds %d MiB", MaxInstallArchiveBytes>>20)
	}

	archive, err := zip.NewReader(readerAt, size)
	if err != nil {
		return Module{}, fmt.Errorf("invalid module archive: %w", err)
	}
	if err := os.MkdirAll(m.rootDir, 0755); err != nil {
		return Module{}, err
	}

	tmpDir, err := os.MkdirTemp(m.rootDir, ".install-*")
	if err != nil {
		return Module{}, err
	}
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	if err := extractArchive(archive, tmpDir); err != nil {
		return Module{}, err
	}

	sourceDir, err := findArchiveManifestDir(tmpDir)
	if err != nil {
		return Module{}, err
	}
	manifest, err := readArchiveManifest(sourceDir)
	if err != nil {
		return Module{}, err
	}
	if err := manifest.Validate(); err != nil {
		return Module{}, err
	}

	targetDir, err := safeModuleTargetDir(m.rootDir, manifest.ID)
	if err != nil {
		return Module{}, err
	}

	backupDir := ""
	if _, err := os.Stat(targetDir); err == nil {
		backupDir = fmt.Sprintf("%s.bak-%d", targetDir, time.Now().UnixNano())
		if err := os.Rename(targetDir, backupDir); err != nil {
			return Module{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Module{}, err
	}

	if err := os.Rename(sourceDir, targetDir); err != nil {
		if backupDir != "" {
			_ = os.RemoveAll(targetDir)
			_ = os.Rename(backupDir, targetDir)
		}
		return Module{}, err
	}
	if backupDir != "" {
		_ = os.RemoveAll(backupDir)
	}
	if sameCleanPath(sourceDir, tmpDir) {
		cleanupTemp = false
	}

	if err := m.Scan(); err != nil {
		return Module{}, err
	}
	module, ok := m.Get(manifest.ID)
	if !ok {
		return Module{}, errors.New("installed module was not found after scan")
	}
	return module, nil
}

func (m *Manager) loadState() (stateFile, error) {
	state := stateFile{Modules: map[string]moduleState{}}
	data, err := os.ReadFile(m.statePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return state, nil
		}
		return state, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return state, nil
	}
	if err := common.Unmarshal(data, &state); err != nil {
		return stateFile{}, err
	}
	if state.Modules == nil {
		state.Modules = map[string]moduleState{}
	}
	return state, nil
}

func (m *Manager) saveStateLocked() error {
	if err := os.MkdirAll(m.rootDir, 0755); err != nil {
		return err
	}
	data, err := common.Marshal(m.state)
	if err != nil {
		return err
	}
	return os.WriteFile(m.statePath(), data, 0644)
}

func (m *Manager) statePath() string {
	return filepath.Join(m.rootDir, "state.json")
}

func extractArchive(archive *zip.Reader, targetDir string) error {
	targetAbs, err := filepath.Abs(targetDir)
	if err != nil {
		return err
	}
	var extracted int64
	for _, file := range archive.File {
		relativePath, err := cleanArchivePath(file.Name)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetAbs, relativePath)
		if err := ensurePathInside(targetAbs, targetPath); err != nil {
			return err
		}

		info := file.FileInfo()
		if info.IsDir() {
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("archive contains unsupported file: %s", file.Name)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		source, err := file.Open()
		if err != nil {
			return err
		}
		target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			_ = source.Close()
			return err
		}

		remaining := maxInstallArchiveExpandedBytes - extracted + 1
		if remaining <= 0 {
			_ = source.Close()
			_ = target.Close()
			return fmt.Errorf("module archive extracted content exceeds %d MiB", maxInstallArchiveExpandedBytes>>20)
		}
		written, copyErr := io.Copy(target, io.LimitReader(source, remaining))
		sourceErr := source.Close()
		targetErr := target.Close()
		extracted += written
		if copyErr != nil {
			return copyErr
		}
		if sourceErr != nil {
			return sourceErr
		}
		if targetErr != nil {
			return targetErr
		}
		if extracted > maxInstallArchiveExpandedBytes {
			return fmt.Errorf("module archive extracted content exceeds %d MiB", maxInstallArchiveExpandedBytes>>20)
		}
	}
	return nil
}

func cleanArchivePath(name string) (string, error) {
	normalized := strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	if normalized == "" {
		return "", errors.New("archive contains empty path")
	}
	if strings.HasPrefix(normalized, "/") {
		return "", fmt.Errorf("archive contains unsafe path: %s", name)
	}
	cleaned := filepath.Clean(filepath.FromSlash(normalized))
	if filepath.IsAbs(cleaned) || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive contains unsafe path: %s", name)
	}
	return cleaned, nil
}

func findArchiveManifestDir(root string) (string, error) {
	candidates := make([]string, 0, 2)
	if regularFileExists(filepath.Join(root, "manifest.json")) {
		candidates = append(candidates, root)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		moduleDir := filepath.Join(root, entry.Name())
		if regularFileExists(filepath.Join(moduleDir, "manifest.json")) {
			candidates = append(candidates, moduleDir)
		}
	}

	switch len(candidates) {
	case 0:
		return "", errors.New("module archive must contain manifest.json")
	case 1:
		return candidates[0], nil
	default:
		return "", errors.New("module archive contains multiple manifest.json files")
	}
}

func readArchiveManifest(moduleDir string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(moduleDir, "manifest.json"))
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := common.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, err
	}
	manifest.ID = strings.TrimSpace(manifest.ID)
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Version = strings.TrimSpace(manifest.Version)
	return manifest, nil
}

func safeModuleTargetDir(rootDir, id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("module id is required")
	}
	if id == "." || id == ".." || strings.ContainsAny(id, `/\`) || filepath.Clean(id) != id {
		return "", fmt.Errorf("module id is not a safe directory name: %s", id)
	}

	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return "", err
	}
	targetDir := filepath.Join(rootAbs, id)
	if err := ensurePathInside(rootAbs, targetDir); err != nil {
		return "", err
	}
	return targetDir, nil
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func ensurePathInside(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("path escapes target directory: %s", target)
	}
	return nil
}

func sameCleanPath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return filepath.Clean(left) == filepath.Clean(right)
	}
	if os.PathSeparator == '\\' {
		return strings.EqualFold(filepath.Clean(leftAbs), filepath.Clean(rightAbs))
	}
	return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}

func roleAllowed(role int, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, value := range allowed {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "guest":
			return true
		case "user", "common":
			if role >= common.RoleCommonUser {
				return true
			}
		case "admin":
			if role >= common.RoleAdminUser {
				return true
			}
		case "root", "super_admin":
			if role >= common.RoleRootUser {
				return true
			}
		}
	}
	return false
}
