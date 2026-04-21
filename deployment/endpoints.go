package deployment

type EndpointSpec struct {
	ID              string `json:"id"`
	Group           string `json:"group"`
	Name            string `json:"name"`
	Method          string `json:"method"`
	Path            string `json:"path"`
	Summary         string `json:"summary"`
	RequestType     string `json:"request_type,omitempty"`
	ResponseType    string `json:"response_type,omitempty"`
	Body            any    `json:"body"`
	ResponseExample any    `json:"response_example,omitempty"`
}

func EndpointSpecs() []EndpointSpec {
	return []EndpointSpec{
		{
			ID:              "health",
			Group:           "Core",
			Name:            "Health",
			Method:          "GET",
			Path:            "/api/v1/health",
			Summary:         "Service liveness",
			ResponseType:    "HealthResult",
			Body:            nil,
			ResponseExample: map[string]string{"status": "ok"},
		},
		{
			ID:           "proxy-nginx",
			Group:        "Proxy",
			Name:         "Install nginx",
			Method:       "POST",
			Path:         "/api/v1/proxy/nginx/install",
			Summary:      "Remote web proxy installer",
			RequestType:  "ProxyInstallRequest",
			ResponseType: "ProxyInstallResult",
			Body: ProxyInstallRequest{
				SSH:             exampleSSHOptions("proxy.example.internal"),
				LocalConfigPath: "",
			},
			ResponseExample: ProxyInstallResult{
				Host:        "proxy.example.internal",
				Name:        "nginx",
				PackageName: "nginx",
				BinaryName:  "nginx",
				ServiceName: "nginx",
				ConfigPath:  "/etc/nginx/nginx.conf",
			},
		},
		{
			ID:           "proxy-haproxy",
			Group:        "Proxy",
			Name:         "Install HAProxy",
			Method:       "POST",
			Path:         "/api/v1/proxy/haproxy/install",
			Summary:      "Remote web proxy installer",
			RequestType:  "ProxyInstallRequest",
			ResponseType: "ProxyInstallResult",
			Body: ProxyInstallRequest{
				SSH:             exampleSSHOptions("proxy.example.internal"),
				LocalConfigPath: "",
			},
			ResponseExample: ProxyInstallResult{
				Host:        "proxy.example.internal",
				Name:        "haproxy",
				PackageName: "haproxy",
				BinaryName:  "haproxy",
				ServiceName: "haproxy",
				ConfigPath:  "/etc/haproxy/haproxy.cfg",
			},
		},
		{
			ID:           "garage-node",
			Group:        "Storage",
			Name:         "Garage node",
			Method:       "POST",
			Path:         "/api/v1/storage/s3/garage/node",
			Summary:      "Deploy a Garage S3 storage node",
			RequestType:  "GarageNodeRequest",
			ResponseType: "GarageNodeResult",
			Body: GarageNodeRequest{
				SSH:               exampleSSHOptions("garage.example.internal"),
				Version:           "v2.2.0",
				BinaryPath:        "/usr/local/bin/garage",
				ConfigPath:        "/etc/garage.toml",
				MetadataDir:       "/var/lib/garage/meta",
				DataDir:           "/var/lib/garage/data",
				DBEngine:          "sqlite",
				ReplicationFactor: 1,
				RPCBindAddr:       "[::]:3901",
				RPCPublicAddr:     "",
				RPCSecret:         "",
				S3APIBindAddr:     "[::]:3900",
				S3Region:          "garage",
				S3RootDomain:      ".s3.local",
				S3WebBindAddr:     "[::]:3902",
				S3WebRootDomain:   ".web.local",
				S3WebIndex:        "index.html",
				K2VAPIBindAddr:    "[::]:3904",
				AdminAPIBindAddr:  "[::]:3903",
				AdminToken:        "",
				MetricsToken:      "",
				LogLevel:          "garage=info",
			},
			ResponseExample: GarageNodeResult{
				Host:          "garage.example.internal",
				BinaryPath:    "/usr/local/bin/garage",
				ConfigPath:    "/etc/garage.toml",
				ServiceName:   "garage",
				RPCPublicAddr: "garage.example.internal:3901",
				S3Endpoint:    "http://garage.example.internal:3900",
				AdminEndpoint: "http://garage.example.internal:3903",
				AdminToken:    "generated-admin-token",
				MetricsToken:  "generated-metrics-token",
			},
		},
		{
			ID:           "garage-token",
			Group:        "Storage",
			Name:         "Garage S3 token",
			Method:       "POST",
			Path:         "/api/v1/storage/s3/garage/token",
			Summary:      "Create bucket credentials",
			RequestType:  "GarageTokenRequest",
			ResponseType: "GarageTokenResult",
			Body: GarageTokenRequest{
				SSH:                exampleSSHOptions("garage.example.internal"),
				BucketName:         "app-assets",
				KeyName:            "app-assets-key",
				CreateBucket:       BoolPtr(true),
				AllowCreateBuckets: BoolPtr(false),
				AllowRead:          BoolPtr(true),
				AllowWrite:         BoolPtr(true),
				AllowOwner:         BoolPtr(true),
				BinaryPath:         "/usr/local/bin/garage",
				ConfigPath:         "/etc/garage.toml",
				S3Endpoint:         "",
				LayoutZone:         "dc1",
				LayoutCapacity:     "100G",
			},
			ResponseExample: GarageTokenResult{
				Host:              "garage.example.internal",
				S3Endpoint:        "http://garage.example.internal:3900",
				BucketName:        "app-assets",
				KeyName:           "app-assets-key",
				AccessKeyID:       "generated-access-key",
				SecretAccessKey:   "generated-secret-key",
				MCAliasSetCommand: "mc alias set garage http://garage.example.internal:3900 generated-access-key generated-secret-key",
			},
		},
		{
			ID:           "valkey-install",
			Group:        "Database",
			Name:         "Install Valkey",
			Method:       "POST",
			Path:         "/api/v1/database/valkey/install",
			Summary:      "Install and configure Valkey for remote access",
			RequestType:  "ValkeyInstallRequest",
			ResponseType: "ValkeyInstallResult",
			Body: ValkeyInstallRequest{
				SSH:      exampleSSHOptions("cache.example.internal"),
				Username: "app",
				Password: "secret",
				Bind:     "0.0.0.0",
				Port:     6379,
				ACLFile:  "",
			},
			ResponseExample: ValkeyInstallResult{
				Host:     "cache.example.internal",
				Port:     6379,
				Username: "app",
				URI:      "redis://app:secret@cache.example.internal:6379",
			},
		},
		{
			ID:           "mariadb-install",
			Group:        "Database",
			Name:         "Install MariaDB",
			Method:       "POST",
			Path:         "/api/v1/database/mariadb/install",
			Summary:      "Install and configure MariaDB for remote access",
			RequestType:  "MariaDBInstallRequest",
			ResponseType: "MariaDBInstallResult",
			Body: MariaDBInstallRequest{
				SSH:          exampleSSHOptions("db.example.internal"),
				DatabaseName: "appdb",
				Username:     "app",
				Password:     "secret",
				Bind:         "0.0.0.0",
				Port:         3306,
			},
			ResponseExample: MariaDBInstallResult{
				Host:         "db.example.internal",
				Port:         3306,
				DatabaseName: "appdb",
				Username:     "app",
				URI:          "mysql://app:secret@db.example.internal:3306/appdb",
			},
		},
	}
}

func exampleSSHOptions(host string) SSHOptions {
	return SSHOptions{
		Host:             host,
		User:             "ubuntu",
		PrivateKeyBase64: "base64-encoded-private-key",
		UseAgent:         true,
		Port:             22,
	}
}

func BoolPtr(value bool) *bool {
	return &value
}
