package extension

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	RuntimeTypeHTTP    = "http"
	RuntimeTypeStatic  = "static"
	DefaultRootDir     = "data/modules"
	DefaultStaticDir   = "public"
	RenderTypeNative   = "native"
	NativeSDKV1        = "v1"
	CapabilityUINative = "ui.native"
)

type Manifest struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Version       string                   `json:"version"`
	Description   string                   `json:"description,omitempty"`
	Author        string                   `json:"author,omitempty"`
	Host          HostCompat               `json:"host,omitempty"`
	Runtime       Runtime                  `json:"runtime"`
	UI            UIContribution           `json:"ui,omitempty"`
	Permissions   PermissionConfig         `json:"permissions,omitempty"`
	Notifications NotificationContribution `json:"notifications,omitempty"`
}

type HostCompat struct {
	Min string `json:"min,omitempty"`
	Max string `json:"max,omitempty"`
}

type Runtime struct {
	Type       string `json:"type"`
	BaseURL    string `json:"base_url,omitempty"`
	HealthPath string `json:"health_path,omitempty"`
	StaticDir  string `json:"static_dir,omitempty"`
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
	Key    string      `json:"key"`
	Title  string      `json:"title,omitempty"`
	Path   string      `json:"path"`
	Embed  bool        `json:"embed"`
	Render *PageRender `json:"render,omitempty"`
}

type PageRender struct {
	Type    string              `json:"type"`
	SDK     string              `json:"sdk"`
	Targets NativeRenderTargets `json:"targets"`
}

type NativeRenderTargets struct {
	Default NativeRenderTarget `json:"default"`
	Classic NativeRenderTarget `json:"classic"`
}

type NativeRenderTarget struct {
	Entry  string   `json:"entry"`
	Styles []string `json:"styles,omitempty"`
}

type PermissionConfig struct {
	Roles        []string `json:"roles,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type Module struct {
	Manifest
	Enabled       bool   `json:"enabled"`
	AssetRevision string `json:"asset_revision,omitempty"`
	Path          string `json:"path,omitempty"`
	Error         string `json:"error,omitempty"`
}

type PublicModule struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Version       string                   `json:"version"`
	Description   string                   `json:"description,omitempty"`
	Author        string                   `json:"author,omitempty"`
	Host          HostCompat               `json:"host,omitempty"`
	Runtime       PublicRuntime            `json:"runtime,omitempty"`
	UI            UIContribution           `json:"ui,omitempty"`
	Permissions   PermissionConfig         `json:"permissions,omitempty"`
	Notifications NotificationContribution `json:"notifications,omitempty"`
	Enabled       bool                     `json:"enabled"`
	AssetRevision string                   `json:"asset_revision,omitempty"`
	Error         string                   `json:"error,omitempty"`
}

type PublicRuntime struct {
	Type       string `json:"type"`
	HealthPath string `json:"health_path,omitempty"`
	StaticDir  string `json:"static_dir,omitempty"`
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

	switch m.Runtime.Type {
	case RuntimeTypeHTTP:
		parsed, err := url.Parse(strings.TrimSpace(m.Runtime.BaseURL))
		if err != nil || parsed == nil || parsed.Host == "" {
			return errors.New("runtime.base_url is invalid")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return errors.New("runtime.base_url only supports http or https")
		}
	case RuntimeTypeStatic:
		staticDir := strings.TrimSpace(m.Runtime.StaticDir)
		if staticDir == "" {
			staticDir = DefaultStaticDir
		}
		staticDir = filepath.Clean(filepath.FromSlash(staticDir))
		if filepath.IsAbs(staticDir) || staticDir == "." || staticDir == ".." || strings.HasPrefix(staticDir, ".."+string(filepath.Separator)) || hasDotPathSegment(filepath.ToSlash(staticDir)) {
			return errors.New("runtime.static_dir is invalid")
		}
		m.Runtime.StaticDir = filepath.ToSlash(staticDir)
	default:
		return errors.New("only http and static runtimes are supported")
	}
	pageKeys := make(map[string]struct{}, len(m.UI.Pages))
	for _, page := range m.UI.Pages {
		if !isValidPageKey(page.Key) {
			return errors.New("ui.pages.key must contain only letters, numbers, - or _")
		}
		if _, exists := pageKeys[page.Key]; exists {
			return errors.New("ui.pages.key must be unique")
		}
		pageKeys[page.Key] = struct{}{}
		if err := validatePagePath(page.Path); err != nil {
			return fmt.Errorf("ui.pages[%s].path is invalid: %w", page.Key, err)
		}
		if m.Runtime.Type == RuntimeTypeStatic && hasDotPathSegment(page.Path) {
			return fmt.Errorf("ui.pages[%s].path is invalid: static paths must not contain dot-prefixed segments", page.Key)
		}
		if err := m.validatePageRender(page); err != nil {
			return err
		}
	}
	for _, nav := range m.UI.Nav {
		if strings.TrimSpace(nav.Title) == "" {
			return errors.New("ui.nav.title is required")
		}
		if strings.TrimSpace(nav.Page) == "" {
			return errors.New("ui.nav.page is required")
		}
		if _, exists := pageKeys[nav.Page]; !exists {
			return errors.New("ui.nav.page must reference ui.pages.key")
		}
	}
	if err := m.validateContributions(); err != nil {
		return err
	}
	return nil
}

func isValidPageKey(value string) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

func validatePagePath(value string) error {
	if value == "" || value != strings.TrimSpace(value) || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return errors.New("path must be an absolute module path")
	}
	if strings.ContainsAny(value, `\?#%`) {
		return errors.New("path must not contain backslashes, percent encoding, query or fragment components")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return errors.New("path must not contain control characters")
		}
	}
	if path.Clean(value) != value {
		return errors.New("path must be canonical and must not contain dot segments, duplicate separators or a trailing separator")
	}
	if value == "/" {
		return nil
	}
	for _, segment := range strings.Split(strings.TrimPrefix(value, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("path must not contain empty or dot segments")
		}
	}
	return nil
}

func (m *Manifest) validatePageRender(page Page) error {
	if page.Render == nil {
		return nil
	}
	render := page.Render
	if render.Type != RenderTypeNative {
		return fmt.Errorf("ui.pages[%s].render.type only supports native", page.Key)
	}
	if m.Runtime.Type != RuntimeTypeStatic {
		return fmt.Errorf("ui.pages[%s].render native requires static runtime", page.Key)
	}
	if render.SDK != NativeSDKV1 {
		return fmt.Errorf("ui.pages[%s].render.sdk must be v1", page.Key)
	}
	if !hasPermissionCapability(m.Permissions.Capabilities, CapabilityUINative) {
		return fmt.Errorf("ui.pages[%s].render native requires ui.native capability", page.Key)
	}
	if err := validateNativeRenderTarget(page.Key, "default", render.Targets.Default); err != nil {
		return err
	}
	return validateNativeRenderTarget(page.Key, "classic", render.Targets.Classic)
}

func validateNativeRenderTarget(pageKey, targetName string, target NativeRenderTarget) error {
	if err := validateNativeAssetPath(target.Entry, ".js", ".mjs"); err != nil {
		return fmt.Errorf("ui.pages[%s].render.targets.%s.entry is invalid: %w", pageKey, targetName, err)
	}
	for index, style := range target.Styles {
		if err := validateNativeAssetPath(style, ".css"); err != nil {
			return fmt.Errorf("ui.pages[%s].render.targets.%s.styles[%d] is invalid: %w", pageKey, targetName, index, err)
		}
	}
	return nil
}

func validateNativeAssetPath(value string, extensions ...string) error {
	if value == "" || value != strings.TrimSpace(value) || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return errors.New("path must be an absolute module path")
	}
	if strings.ContainsAny(value, "\\:?#\x00") || path.Clean(value) != value {
		return errors.New("path must not contain traversal, query or fragment components")
	}
	for _, segment := range strings.Split(strings.TrimPrefix(value, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("path must not contain traversal components")
		}
	}
	extension := path.Ext(value)
	for _, allowed := range extensions {
		if extension == allowed {
			return nil
		}
	}
	return fmt.Errorf("path extension must be %s", strings.Join(extensions, " or "))
}

func (m *Manifest) validateContributions() error {
	seen := make(map[string]struct{}, len(m.Permissions.Capabilities))
	notificationCapabilities := make([]string, 0, 1)
	for index, capability := range m.Permissions.Capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" {
			return errors.New("permissions.capabilities cannot contain an empty value")
		}
		if _, exists := seen[capability]; exists {
			return fmt.Errorf("duplicate permission capability: %s", capability)
		}
		seen[capability] = struct{}{}
		m.Permissions.Capabilities[index] = capability
		switch capability {
		case CapabilityUINative:
		case CapabilityNotificationEventsPublish:
			notificationCapabilities = append(notificationCapabilities, capability)
		default:
			return fmt.Errorf("unsupported permission capability: %s", capability)
		}
	}

	capabilities := m.Permissions.Capabilities
	m.Permissions.Capabilities = notificationCapabilities
	err := m.validateNotificationContribution()
	m.Permissions.Capabilities = capabilities
	return err
}

func hasPermissionCapability(capabilities []string, expected string) bool {
	for _, capability := range capabilities {
		if strings.TrimSpace(capability) == expected {
			return true
		}
	}
	return false
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
			StaticDir:  m.Runtime.StaticDir,
		},
		UI:            m.UI,
		Permissions:   m.Permissions,
		Notifications: m.Notifications,
		Enabled:       m.Enabled,
		AssetRevision: m.AssetRevision,
	}
	if includeAdminFields {
		result.Error = m.Error
	}
	return result
}
