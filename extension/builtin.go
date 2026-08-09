package extension

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

//go:embed builtin/*
var builtinModules embed.FS

func installBuiltinModules(rootDir string) error {
	entries, err := fs.ReadDir(builtinModules, "builtin")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		moduleID := strings.TrimSpace(entry.Name())
		targetDir, err := safeModuleTargetDir(rootDir, moduleID)
		if err != nil {
			return err
		}
		shouldInstall, err := shouldInstallBuiltinModule(moduleID, targetDir)
		if err != nil {
			return fmt.Errorf("inspect builtin module %s: %w", moduleID, err)
		}
		if !shouldInstall {
			continue
		}
		if err := os.RemoveAll(targetDir); err != nil {
			return fmt.Errorf("clear builtin module %s: %w", moduleID, err)
		}
		if err := copyBuiltinModule("builtin/"+moduleID, targetDir); err != nil {
			return fmt.Errorf("install builtin module %s: %w", moduleID, err)
		}
	}
	return nil
}

func readBuiltinManifest(moduleID string) (Manifest, error) {
	manifestBytes, err := builtinModules.ReadFile("builtin/" + moduleID + "/manifest.json")
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := common.Unmarshal(manifestBytes, &manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func shouldInstallBuiltinModule(moduleID string, targetDir string) (bool, error) {
	builtinManifest, err := readBuiltinManifest(moduleID)
	if err != nil {
		return false, err
	}
	targetManifestPath := filepath.Join(targetDir, "manifest.json")
	if !regularFileExists(targetManifestPath) {
		return true, nil
	}
	targetBytes, err := os.ReadFile(targetManifestPath)
	if err != nil {
		return false, err
	}
	var installedManifest Manifest
	if err := common.Unmarshal(targetBytes, &installedManifest); err != nil {
		return true, nil
	}
	if strings.TrimSpace(installedManifest.ID) != moduleID {
		return false, fmt.Errorf("installed module id %q does not match builtin id", installedManifest.ID)
	}
	return strings.TrimSpace(installedManifest.Version) != strings.TrimSpace(builtinManifest.Version), nil
}

func copyBuiltinModule(sourceRoot string, targetRoot string) error {
	return fs.WalkDir(builtinModules, sourceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(filepath.FromSlash(sourceRoot), filepath.FromSlash(path))
		if err != nil {
			return err
		}
		if relativePath == "." {
			return os.MkdirAll(targetRoot, 0755)
		}
		targetPath := filepath.Join(targetRoot, relativePath)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}
		data, err := builtinModules.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, 0644)
	})
}

func validateBuiltinModules() error {
	entries, err := fs.ReadDir(builtinModules, "builtin")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, err := readBuiltinManifest(entry.Name())
		if err != nil {
			return err
		}
		if err := manifest.Validate(); err != nil {
			return err
		}
	}
	return nil
}
