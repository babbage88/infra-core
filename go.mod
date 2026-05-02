// infractl:name=infra-core
// infractl:description=Shared deployment and infrastructure automation primitives used by registerable user applications.
// infractl:repository_url=https://github.com/babbage88/infra-core
// infractl:manifest_path=go.mod
// infractl:deploy_kind=library
// infractl:package_manager=go
// infractl:registerable=false
// infractl:build_config={"role":"support-library"}
module github.com/babbage88/infra-core

go 1.26.2

require (
	github.com/cloudflare/cloudflare-go v0.116.0
	github.com/go-acme/lego/v4 v4.35.2
	github.com/goccy/go-yaml v1.19.2
	github.com/gorilla/websocket v1.5.3
	github.com/joho/godotenv v1.5.1
	github.com/minio/minio-go/v7 v7.0.100
	github.com/pkg/sftp v1.13.10
	golang.org/x/term v0.42.0
)

require (
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/klauspost/crc32 v1.3.0 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/tinylib/msgp v1.6.1 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
)

require (
	github.com/babbage88/goph/v2 v2.0.1
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-ini/ini v1.67.0 // indirect
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/google/go-querystring v1.2.0 // indirect
	github.com/google/uuid v1.6.0
	github.com/kevinburke/ssh_config v1.2.0
	github.com/klauspost/compress v1.18.2 // indirect
	github.com/klauspost/cpuid/v2 v2.2.11 // indirect
	github.com/lib/pq v1.12.3
	github.com/miekg/dns v1.1.72 // indirect
	github.com/minio/crc64nvme v1.1.1 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/rs/xid v1.6.0 // indirect
	github.com/skeema/knownhosts v1.3.1
	golang.org/x/crypto v0.50.0
	golang.org/x/mod v0.35.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.44.0 // indirect
)
