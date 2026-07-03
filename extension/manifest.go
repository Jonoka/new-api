package extension

import (
	"errors"
	"net/url"
	"strings"
)

const (
	RuntimeTypeHTTP = "http"
	DefaultRootDir  = "data/modules"
)

type Manifest struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Version     string           `json:"version"`
	Description string           `json:"description,omitempty"`
	Author      string           `json:"author,omitempty"`
	Host        HostCompat       `json:"host,omitempty"`
	Runtime     Runtime          `json:"runtime"`
	UI          UIContribution   `json:"ui,omitempty"`
	Permissions PermissionConfig `json:"permissions,omitempty"`
}

type HostCompat struct {
	Min string `json:"min,omitempty"`
	Max string `json:"max,omitempty"`
}

type Runtime struct {
	Type       string `json:"type"`
	BaseURL    string `json:"base_url"`
	HealthPath string `json:"health_path,omitempty"`
}

type UIContribution struct {
	Nav   []NavItem `json:"nav,omitempty"`
	Pages []Page    `json:"pages,omitempty"`
}

type NavItem struct {
	Title   string `json:"title"`
	Page    string `json:"page"`
	Icon    string `json:"icon,omitempty"`
	Section string `json:"section,omitempty"`
	Order   int    `json:"order,omitempty"`
}

type Page struct {
	Key   string `json:"key"`
	Title string `json:"title,omitempty"`
	Path  string `json:"path"`
	Embed bool   `json:"embed"`
}

type PermissionConfig struct {
	Roles []string `json:"roles,omitempty"`
}

type Module struct {
	Manifest
	Enabled bool   `json:"enabled"`
	Path    string `json:"path,omitempty"`
	Error   string `json:"error,omitempty"`
}

type PublicModule struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Version     string           `json:"version"`
	Description string           `json:"description,omitempty"`
	Author      string           `json:"author,omitempty"`
	Host        HostCompat       `json:"host,omitempty"`
	Runtime     PublicRuntime    `json:"runtime,omitempty"`
	UI          UIContribution   `json:"ui,omitempty"`
	Permissions PermissionConfig `json:"permissions,omitempty"`
	Enabled     bool             `json:"enabled"`
	Error       string           `json:"error,omitempty"`
}

type PublicRuntime struct {
	Type       string `json:"type"`
	HealthPath string `json:"health_path,omitempty"`
}

func (m *Manifest) Validate() error {
	if strings.TrimSpace(m.ID) == "" {
		return errors.New("module id is required")
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("module name is required")
	}
	if strings.TrimSpace(m.Version) == "" {
		return errors.New("module version is required")
	}
	m.Runtime.Type = strings.TrimSpace(m.Runtime.Type)
	m.Runtime.BaseURL = strings.TrimSpace(m.Runtime.BaseURL)
	if m.Runtime.Type == "" {
		m.Runtime.Type = RuntimeTypeHTTP
	}
	if m.Runtime.Type != RuntimeTypeHTTP {
		return errors.New("only http runtime is supported")
	}
	parsed, err := url.Parse(strings.TrimSpace(m.Runtime.BaseURL))
	if err != nil || parsed == nil || parsed.Host == "" {
		return errors.New("runtime.base_url is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("runtime.base_url only supports http or https")
	}
	for _, page := range m.UI.Pages {
		if strings.TrimSpace(page.Key) == "" {
			return errors.New("ui.pages.key is required")
		}
		if !strings.HasPrefix(page.Path, "/") {
			return errors.New("ui.pages.path must start with /")
		}
	}
	for _, nav := range m.UI.Nav {
		if strings.TrimSpace(nav.Title) == "" {
			return errors.New("ui.nav.title is required")
		}
		if strings.TrimSpace(nav.Page) == "" {
			return errors.New("ui.nav.page is required")
		}
	}
	return nil
}

func (m Module) Public(includeAdminFields bool) PublicModule {
	result := PublicModule{
		ID:          m.ID,
		Name:        m.Name,
		Version:     m.Version,
		Description: m.Description,
		Author:      m.Author,
		Host:        m.Host,
		Runtime: PublicRuntime{
			Type:       m.Runtime.Type,
			HealthPath: m.Runtime.HealthPath,
		},
		UI:          m.UI,
		Permissions: m.Permissions,
		Enabled:     m.Enabled,
	}
	if includeAdminFields {
		result.Error = m.Error
	}
	return result
}
