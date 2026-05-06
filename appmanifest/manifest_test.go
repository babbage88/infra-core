package appmanifest

import "testing"

func TestParsePackageJSON(t *testing.T) {
	content := []byte(`{
		"name": "infractl-ui",
		"description": "Frontend UI",
		"dependencies": {
			"react": "^19.0.0"
		},
		"devDependencies": {
			"vite": "^6.0.0"
		},
		"infractl": {
			"name": "infractl-ui",
			"repositoryUrl": "https://github.com/babbage88/infractl-ui",
			"deployConfig": {"outputDir":"builds/dev"},
			"infraDependencies": [
				{"dependencyType":"host_server_type","dependencyName":"Application Server"}
			]
		}
	}`)

	manifest, err := ParsePackageJSON(content)
	if err != nil {
		t.Fatalf("ParsePackageJSON returned error: %v", err)
	}
	if manifest.Name != "infractl-ui" {
		t.Fatalf("expected manifest name infractl-ui, got %q", manifest.Name)
	}
	if manifest.PackageName != "infractl-ui" {
		t.Fatalf("expected package name infractl-ui, got %q", manifest.PackageName)
	}
	if manifest.DeployKind != "nginx_static_site" {
		t.Fatalf("expected deploy kind nginx_static_site, got %q", manifest.DeployKind)
	}
	if len(manifest.InfraDependencies) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(manifest.InfraDependencies))
	}
}

func TestParseGoMod(t *testing.T) {
	content := []byte(`// infractl:name=go-infra
// infractl:repository_url=https://github.com/babbage88/go-infra
// infractl:deploy_kind=systemd_service
// infractl:dependency=host_server_type:Application Server
// infractl:dependency=platform_type:Linux VPS
module github.com/babbage88/go-infra
`)

	manifest, err := ParseGoMod(content)
	if err != nil {
		t.Fatalf("ParseGoMod returned error: %v", err)
	}
	if manifest.Name != "go-infra" {
		t.Fatalf("expected manifest name go-infra, got %q", manifest.Name)
	}
	if manifest.ModuleName != "github.com/babbage88/go-infra" {
		t.Fatalf("expected module name github.com/babbage88/go-infra, got %q", manifest.ModuleName)
	}
	if manifest.DeployKind != "systemd_service" {
		t.Fatalf("expected deploy kind systemd_service, got %q", manifest.DeployKind)
	}
	if len(manifest.InfraDependencies) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(manifest.InfraDependencies))
	}
}

func TestParseProjectJSON(t *testing.T) {
	content := []byte(`{
		"name": "portal-web",
		"projectType": "application",
		"sourceRoot": "apps/portal-web/src",
		"targets": {
			"build": {
				"executor": "@nx/react:build"
			}
		}
	}`)

	manifest, err := ParseProjectJSON(content)
	if err != nil {
		t.Fatalf("ParseProjectJSON returned error: %v", err)
	}
	if manifest.Name != "portal-web" {
		t.Fatalf("expected manifest name portal-web, got %q", manifest.Name)
	}
	if manifest.DeployKind != "nginx_static_site" {
		t.Fatalf("expected deploy kind nginx_static_site, got %q", manifest.DeployKind)
	}
}

func TestParseCargoToml(t *testing.T) {
	content := []byte(`[package]
name = "api-server"
description = "Rust API"
repository = "https://github.com/example/api-server"

[dependencies]
axum = "0.8"
tokio = { version = "1", features = ["rt-multi-thread"] }
`)

	manifest, err := ParseCargoToml(content)
	if err != nil {
		t.Fatalf("ParseCargoToml returned error: %v", err)
	}
	if manifest.Name != "api-server" {
		t.Fatalf("expected manifest name api-server, got %q", manifest.Name)
	}
	if manifest.PackageManager != "cargo" {
		t.Fatalf("expected package manager cargo, got %q", manifest.PackageManager)
	}
	if manifest.DeployKind != "systemd_service" {
		t.Fatalf("expected deploy kind systemd_service, got %q", manifest.DeployKind)
	}
}

func TestParseCsproj(t *testing.T) {
	content := []byte(`<Project Sdk="Microsoft.NET.Sdk.Web">
  <PropertyGroup>
    <AssemblyName>OpsApi</AssemblyName>
    <RootNamespace>Compunity.OpsApi</RootNamespace>
    <OutputType>Exe</OutputType>
    <TargetFramework>net8.0</TargetFramework>
  </PropertyGroup>
</Project>`)

	manifest, err := ParseCsproj("src/OpsApi/OpsApi.csproj", content)
	if err != nil {
		t.Fatalf("ParseCsproj returned error: %v", err)
	}
	if manifest.Name != "OpsApi" {
		t.Fatalf("expected manifest name OpsApi, got %q", manifest.Name)
	}
	if manifest.ModuleName != "Compunity.OpsApi" {
		t.Fatalf("expected module name Compunity.OpsApi, got %q", manifest.ModuleName)
	}
	if manifest.PackageManager != "dotnet" {
		t.Fatalf("expected package manager dotnet, got %q", manifest.PackageManager)
	}
}

func TestParseManifestFileDispatch(t *testing.T) {
	testCases := []struct {
		name         string
		manifestPath string
		content      []byte
		sourceKind   string
	}{
		{
			name:         "package json",
			manifestPath: "package.json",
			content:      []byte(`{"name":"web","dependencies":{"react":"^19.0.0"}}`),
			sourceKind:   "package.json",
		},
		{
			name:         "go mod",
			manifestPath: "go.mod",
			content:      []byte("module github.com/example/service\n"),
			sourceKind:   "go.mod",
		},
		{
			name:         "project json",
			manifestPath: "apps/site/project.json",
			content:      []byte(`{"name":"site","targets":{"build":{"executor":"@nx/web:build"}}}`),
			sourceKind:   "project.json",
		},
		{
			name:         "csproj",
			manifestPath: "src/Api/Api.csproj",
			content:      []byte(`<Project><PropertyGroup><AssemblyName>Api</AssemblyName></PropertyGroup></Project>`),
			sourceKind:   "csproj",
		},
		{
			name:         "cargo",
			manifestPath: "Cargo.toml",
			content:      []byte("[package]\nname = \"api\"\n"),
			sourceKind:   "cargo.toml",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			manifest, err := ParseManifestFile(tc.manifestPath, tc.content)
			if err != nil {
				t.Fatalf("ParseManifestFile returned error: %v", err)
			}
			if manifest.SourceKind != tc.sourceKind {
				t.Fatalf("expected source kind %q, got %q", tc.sourceKind, manifest.SourceKind)
			}
			if manifest.ManifestPath != tc.manifestPath {
				t.Fatalf("expected manifest path %q, got %q", tc.manifestPath, manifest.ManifestPath)
			}
		})
	}
}
