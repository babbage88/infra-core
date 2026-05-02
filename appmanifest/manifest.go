package appmanifest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type InfraDependency struct {
	DependencyType string                 `json:"dependencyType"`
	DependencyName string                 `json:"dependencyName"`
	Config         map[string]interface{} `json:"config,omitempty"`
}

type Manifest struct {
	Name              string                 `json:"name"`
	Description       string                 `json:"description,omitempty"`
	RepositoryURL     string                 `json:"repositoryUrl"`
	ManifestPath      string                 `json:"manifestPath,omitempty"`
	SourceKind        string                 `json:"sourceKind,omitempty"`
	ModuleName        string                 `json:"moduleName,omitempty"`
	PackageName       string                 `json:"packageName,omitempty"`
	PackageManager    string                 `json:"packageManager,omitempty"`
	DeployKind        string                 `json:"deployKind"`
	Registerable      bool                   `json:"registerable"`
	DeployConfig      map[string]interface{} `json:"deployConfig,omitempty"`
	BuildConfig       map[string]interface{} `json:"buildConfig,omitempty"`
	InfraDependencies []InfraDependency      `json:"infraDependencies,omitempty"`
}

type packageJSONDocument struct {
	Name     string   `json:"name"`
	Infractl Manifest `json:"infractl"`
}

func ParsePackageJSON(content []byte) (Manifest, error) {
	var doc packageJSONDocument
	if err := json.Unmarshal(content, &doc); err != nil {
		return Manifest{}, fmt.Errorf("parse package.json: %w", err)
	}
	manifest := doc.Infractl
	if manifest.Name == "" {
		manifest.Name = doc.Name
	}
	if manifest.PackageName == "" {
		manifest.PackageName = doc.Name
	}
	if manifest.SourceKind == "" {
		manifest.SourceKind = "package.json"
	}
	if !manifest.Registerable {
		manifest.Registerable = true
	}
	return manifest, nil
}

func ParseGoMod(content []byte) (Manifest, error) {
	manifest := Manifest{
		Registerable: true,
		SourceKind:   "go.mod",
	}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "module "):
			manifest.ModuleName = strings.TrimSpace(strings.TrimPrefix(line, "module "))
		case strings.HasPrefix(line, "// infractl:"):
			payload := strings.TrimSpace(strings.TrimPrefix(line, "// infractl:"))
			key, value, ok := strings.Cut(payload, "=")
			if !ok {
				continue
			}
			applyGoModManifestKV(&manifest, strings.TrimSpace(key), strings.TrimSpace(value))
		}
	}
	if err := scanner.Err(); err != nil {
		return Manifest{}, fmt.Errorf("scan go.mod: %w", err)
	}
	if manifest.Name == "" && manifest.ModuleName != "" {
		parts := strings.Split(manifest.ModuleName, "/")
		manifest.Name = parts[len(parts)-1]
	}
	return manifest, nil
}

func applyGoModManifestKV(manifest *Manifest, key, value string) {
	switch key {
	case "name":
		manifest.Name = value
	case "description":
		manifest.Description = value
	case "repository_url":
		manifest.RepositoryURL = value
	case "manifest_path":
		manifest.ManifestPath = value
	case "deploy_kind":
		manifest.DeployKind = value
	case "package_manager":
		manifest.PackageManager = value
	case "registerable":
		manifest.Registerable = strings.EqualFold(value, "true")
	case "build_config":
		manifest.BuildConfig = parseJSONMap(value)
	case "deploy_config":
		manifest.DeployConfig = parseJSONMap(value)
	case "dependency":
		kind, name, ok := strings.Cut(value, ":")
		if !ok {
			return
		}
		manifest.InfraDependencies = append(manifest.InfraDependencies, InfraDependency{
			DependencyType: strings.TrimSpace(kind),
			DependencyName: strings.TrimSpace(name),
		})
	}
}

func parseJSONMap(value string) map[string]interface{} {
	parsed := map[string]interface{}{}
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return map[string]interface{}{
			"_raw": value,
		}
	}
	return parsed
}
