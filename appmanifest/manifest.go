package appmanifest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"path/filepath"
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
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Private      bool           `json:"private"`
	Scripts      map[string]any `json:"scripts"`
	Dependencies map[string]any `json:"dependencies"`
	DevDeps      map[string]any `json:"devDependencies"`
	Infractl     Manifest       `json:"infractl"`
}

type genericProjectJSONDocument struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	ProjectType string         `json:"projectType"`
	SourceRoot  string         `json:"sourceRoot"`
	Targets     map[string]any `json:"targets"`
	Infractl    Manifest       `json:"infractl"`
}

type csprojDocument struct {
	XMLName      xml.Name      `xml:"Project"`
	PropertySets []propertySet `xml:"PropertyGroup"`
}

type propertySet struct {
	AssemblyName     string `xml:"AssemblyName"`
	RootNamespace    string `xml:"RootNamespace"`
	OutputType       string `xml:"OutputType"`
	TargetFramework  string `xml:"TargetFramework"`
	TargetFrameworks string `xml:"TargetFrameworks"`
}

func ParseManifestFile(manifestPath string, content []byte) (Manifest, error) {
	base := strings.ToLower(filepath.Base(manifestPath))

	switch {
	case base == "package.json":
		manifest, err := ParsePackageJSON(content)
		if err != nil {
			return Manifest{}, err
		}
		if manifest.ManifestPath == "" {
			manifest.ManifestPath = manifestPath
		}
		return manifest, nil
	case base == "project.json":
		manifest, err := ParseProjectJSON(content)
		if err != nil {
			return Manifest{}, err
		}
		if manifest.ManifestPath == "" {
			manifest.ManifestPath = manifestPath
		}
		return manifest, nil
	case base == "go.mod":
		manifest, err := ParseGoMod(content)
		if err != nil {
			return Manifest{}, err
		}
		if manifest.ManifestPath == "" {
			manifest.ManifestPath = manifestPath
		}
		return manifest, nil
	case base == "cargo.toml":
		manifest, err := ParseCargoToml(content)
		if err != nil {
			return Manifest{}, err
		}
		if manifest.ManifestPath == "" {
			manifest.ManifestPath = manifestPath
		}
		return manifest, nil
	case strings.HasSuffix(base, ".csproj"):
		manifest, err := ParseCsproj(manifestPath, content)
		if err != nil {
			return Manifest{}, err
		}
		if manifest.ManifestPath == "" {
			manifest.ManifestPath = manifestPath
		}
		return manifest, nil
	default:
		return Manifest{}, fmt.Errorf("unsupported manifest file: %s", manifestPath)
	}
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
	if manifest.Description == "" {
		manifest.Description = doc.Description
	}
	if manifest.PackageName == "" {
		manifest.PackageName = doc.Name
	}
	if manifest.SourceKind == "" {
		manifest.SourceKind = "package.json"
	}
	if manifest.PackageManager == "" {
		manifest.PackageManager = "npm"
	}
	if manifest.DeployKind == "" {
		manifest.DeployKind = inferPackageJSONDeployKind(doc)
	}
	if manifest.DeployKind == "nginx_static_site" {
		manifest.InfraDependencies = ensureDependency(
			manifest.InfraDependencies,
			InfraDependency{DependencyType: "platform_type", DependencyName: "Nginx Server"},
		)
	}
	manifest.InfraDependencies = ensureDependency(
		manifest.InfraDependencies,
		InfraDependency{DependencyType: "host_server_type", DependencyName: "Application Server"},
	)
	if !manifest.Registerable {
		manifest.Registerable = true
	}

	return manifest, nil
}

func ParseProjectJSON(content []byte) (Manifest, error) {
	var doc genericProjectJSONDocument
	if err := json.Unmarshal(content, &doc); err != nil {
		return Manifest{}, fmt.Errorf("parse project.json: %w", err)
	}

	manifest := doc.Infractl
	if manifest.Name == "" {
		manifest.Name = doc.Name
	}
	if manifest.Description == "" {
		manifest.Description = doc.Description
	}
	if manifest.SourceKind == "" {
		manifest.SourceKind = "project.json"
	}
	if manifest.PackageManager == "" {
		manifest.PackageManager = "npm"
	}
	if manifest.DeployKind == "" {
		manifest.DeployKind = inferProjectJSONDeployKind(doc)
	}
	manifest.InfraDependencies = ensureDependency(
		manifest.InfraDependencies,
		InfraDependency{DependencyType: "host_server_type", DependencyName: "Application Server"},
	)
	if manifest.DeployKind == "nginx_static_site" {
		manifest.InfraDependencies = ensureDependency(
			manifest.InfraDependencies,
			InfraDependency{DependencyType: "platform_type", DependencyName: "Nginx Server"},
		)
	}
	if !manifest.Registerable {
		manifest.Registerable = true
	}

	return manifest, nil
}

func ParseGoMod(content []byte) (Manifest, error) {
	manifest := Manifest{
		Registerable:   true,
		SourceKind:     "go.mod",
		PackageManager: "go",
		DeployKind:     "systemd_service",
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
	manifest.InfraDependencies = ensureDependency(
		manifest.InfraDependencies,
		InfraDependency{DependencyType: "host_server_type", DependencyName: "Application Server"},
	)
	return manifest, nil
}

func ParseCargoToml(content []byte) (Manifest, error) {
	manifest := Manifest{
		Registerable:   true,
		SourceKind:     "cargo.toml",
		PackageManager: "cargo",
		DeployKind:     "systemd_service",
	}

	currentSection := ""
	hasWebRuntime := false

	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.Trim(line, "[]")
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = trimTomlString(value)

		switch currentSection {
		case "package":
			switch key {
			case "name":
				manifest.Name = value
			case "description":
				manifest.Description = value
			case "repository":
				manifest.RepositoryURL = value
			}
		case "dependencies", "workspace.dependencies":
			switch key {
			case "axum", "actix-web", "rocket", "warp", "poem":
				hasWebRuntime = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Manifest{}, fmt.Errorf("scan cargo.toml: %w", err)
	}

	if hasWebRuntime {
		manifest.DeployKind = "systemd_service"
	}
	manifest.InfraDependencies = ensureDependency(
		manifest.InfraDependencies,
		InfraDependency{DependencyType: "host_server_type", DependencyName: "Application Server"},
	)

	return manifest, nil
}

func ParseCsproj(manifestPath string, content []byte) (Manifest, error) {
	var doc csprojDocument
	if err := xml.Unmarshal(content, &doc); err != nil {
		return Manifest{}, fmt.Errorf("parse csproj: %w", err)
	}

	manifest := Manifest{
		Registerable:   true,
		SourceKind:     "csproj",
		PackageManager: "dotnet",
		DeployKind:     "systemd_service",
		Name:           strings.TrimSuffix(filepath.Base(manifestPath), filepath.Ext(manifestPath)),
	}

	for _, group := range doc.PropertySets {
		if manifest.Name == "" && group.AssemblyName != "" {
			manifest.Name = group.AssemblyName
		}
		if manifest.ModuleName == "" && group.RootNamespace != "" {
			manifest.ModuleName = group.RootNamespace
		}
		if strings.EqualFold(group.OutputType, "Library") {
			manifest.DeployKind = ""
		}
	}

	manifest.InfraDependencies = ensureDependency(
		manifest.InfraDependencies,
		InfraDependency{DependencyType: "host_server_type", DependencyName: "Application Server"},
	)

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

func inferPackageJSONDeployKind(doc packageJSONDocument) string {
	if hasAnyKey(doc.Dependencies, "next", "react", "vite", "@angular/core", "vue", "svelte", "@sveltejs/kit") ||
		hasAnyKey(doc.DevDeps, "vite", "@vitejs/plugin-react", "@angular/cli") {
		if hasAnyKey(doc.Dependencies, "express", "fastify", "koa", "nestjs", "@nestjs/core") {
			return "systemd_service"
		}
		return "nginx_static_site"
	}
	if hasAnyKey(doc.Dependencies, "express", "fastify", "koa", "nestjs", "@nestjs/core") {
		return "systemd_service"
	}
	if scriptContains(doc.Scripts, "build", "vite", "next build", "react-scripts build", "ng build") {
		return "nginx_static_site"
	}
	return ""
}

func inferProjectJSONDeployKind(doc genericProjectJSONDocument) string {
	if strings.EqualFold(doc.ProjectType, "application") {
		if strings.Contains(strings.ToLower(doc.SourceRoot), "app") {
			return "nginx_static_site"
		}
	}

	targetText := strings.ToLower(string(mustJSON(doc.Targets)))
	switch {
	case strings.Contains(targetText, "@nx/web") ||
		strings.Contains(targetText, "@nx/react") ||
		strings.Contains(targetText, "@angular-devkit/build-angular") ||
		strings.Contains(targetText, "vite"):
		return "nginx_static_site"
	case strings.Contains(targetText, "@nx/node") ||
		strings.Contains(targetText, "nest") ||
		strings.Contains(targetText, "node"):
		return "systemd_service"
	default:
		return ""
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

func ensureDependency(existing []InfraDependency, dep InfraDependency) []InfraDependency {
	for _, current := range existing {
		if current.DependencyType == dep.DependencyType && current.DependencyName == dep.DependencyName {
			return existing
		}
	}
	return append(existing, dep)
}

func hasAnyKey(values map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := values[key]; ok {
			return true
		}
	}
	return false
}

func scriptContains(scripts map[string]any, scriptName string, needles ...string) bool {
	raw, ok := scripts[scriptName]
	if !ok {
		return false
	}
	text, ok := raw.(string)
	if !ok {
		return false
	}
	text = strings.ToLower(text)
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func trimTomlString(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimSuffix(trimmed, ",")
	trimmed = strings.Trim(trimmed, `"'`)
	return trimmed
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return encoded
}
