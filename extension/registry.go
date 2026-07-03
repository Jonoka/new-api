package extension

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
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
	return DefaultRootDir
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
