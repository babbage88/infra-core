package appmanifest

import "testing"

func TestParsePackageJSON(t *testing.T) {
	content := []byte(`{
		"name": "infractl-ui",
		"infractl": {
			"name": "infractl-ui",
			"repositoryUrl": "https://github.com/babbage88/infractl-ui",
			"deployKind": "nginx_static_site",
			"deployConfig": {"outputDir":"builds/dev"},
			"infraDependencies": [
				{"dependencyType":"host_server_type","dependencyName":"Application Server"},
				{"dependencyType":"platform_type","dependencyName":"Nginx Server"}
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
