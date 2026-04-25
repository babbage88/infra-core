package deployment

import (
	"encoding/base64"
	"os"
	"testing"
)

func TestMergeMariaDBInstallDefaults(t *testing.T) {
	req := MariaDBInstallRequest{
		DatabaseName: "appdb",
		Username:     "app",
		Password:     "secret",
	}

	merged := MergeMariaDBInstallDefaults(req, DefaultMariaDBInstallRequest())
	if merged.Bind != "0.0.0.0" {
		t.Fatalf("expected default bind, got %q", merged.Bind)
	}
	if merged.Port != 3306 {
		t.Fatalf("expected default port, got %d", merged.Port)
	}
	if merged.DatabaseName != "appdb" {
		t.Fatalf("expected explicit database name to be preserved, got %q", merged.DatabaseName)
	}
}

func TestBuildMariaDBURLEscapesCredentialsAndDatabase(t *testing.T) {
	got := BuildMariaDBURL("db-01", 3306, "app db", "app user", "pa:ss@word")
	want := "mysql://app+user:pa%3Ass%40word@db-01:3306/app+db"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestMergeValkeyInstallDefaults(t *testing.T) {
	req := ValkeyInstallRequest{
		Username: "app",
		Password: "secret",
	}

	merged := MergeValkeyInstallDefaults(req, DefaultValkeyInstallRequest())
	if merged.Bind != "0.0.0.0" {
		t.Fatalf("expected default bind, got %q", merged.Bind)
	}
	if merged.Port != 6379 {
		t.Fatalf("expected default port, got %d", merged.Port)
	}
	if merged.Username != "app" {
		t.Fatalf("expected explicit username to be preserved, got %q", merged.Username)
	}
}

func TestBuildValkeyURLEscapesCredentials(t *testing.T) {
	got := BuildValkeyURL("valkey-01", 6379, "app user", "pa:ss@word")
	want := "redis://app+user:pa%3Ass%40word@valkey-01:6379"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestMergeGarageTokenDefaultsPreservesExplicitFalsePermissions(t *testing.T) {
	req := GarageTokenRequest{
		KeyName:    "app-key",
		BucketName: "app-bucket",
		AllowRead:  BoolPtr(false),
	}

	merged := MergeGarageTokenDefaults(req, DefaultGarageTokenRequest())
	if merged.AllowRead == nil {
		t.Fatal("expected allow_read to be set")
	}
	if *merged.AllowRead {
		t.Fatal("expected explicit allow_read=false to be preserved")
	}
	if merged.AllowWrite == nil || !*merged.AllowWrite {
		t.Fatal("expected omitted allow_write to use default true")
	}
}

func TestMergeGarageTokenDefaultsAllowsClusterWideBucketCreationWithoutBucketPermissions(t *testing.T) {
	req := GarageTokenRequest{
		KeyName:            "cluster-key",
		AllowCreateBuckets: BoolPtr(true),
	}

	merged := MergeGarageTokenDefaults(req, DefaultGarageTokenRequest())
	if merged.AllowRead == nil || *merged.AllowRead {
		t.Fatal("expected allow_read=false when allow_create_buckets is true without a bucket")
	}
	if merged.AllowWrite == nil || *merged.AllowWrite {
		t.Fatal("expected allow_write=false when allow_create_buckets is true without a bucket")
	}
	if merged.AllowOwner == nil || *merged.AllowOwner {
		t.Fatal("expected allow_owner=false when allow_create_buckets is true without a bucket")
	}
}

func TestMergeGarageNodeDefaultsFillsExpectedValues(t *testing.T) {
	req := GarageNodeRequest{
		ReplicationFactor: 3,
		S3Region:          "us-test-1",
	}

	merged := MergeGarageNodeDefaults(req, DefaultGarageNodeRequest())
	if merged.ReplicationFactor != 3 {
		t.Fatalf("expected explicit replication factor to be preserved, got %d", merged.ReplicationFactor)
	}
	if merged.S3Region != "us-test-1" {
		t.Fatalf("expected explicit S3 region to be preserved, got %q", merged.S3Region)
	}
	if merged.Version == "" {
		t.Fatal("expected default Garage version")
	}
	if merged.BinaryPath != "/usr/local/bin/garage" {
		t.Fatalf("expected default binary path, got %q", merged.BinaryPath)
	}
}

func TestGarageBindAddrToAdvertisedUsesHostForWildcardBinds(t *testing.T) {
	if got := garageS3BindAddrToAdvertised("garage-01", "[::]:3900"); got != "garage-01:3900" {
		t.Fatalf("expected host-based S3 endpoint, got %q", got)
	}
	if got := garageBindAddrToAdvertised("garage-01", "[::]:3903"); got != "garage-01:3903" {
		t.Fatalf("expected host-based admin endpoint, got %q", got)
	}
}

func TestPrepareSSHOptionsWritesBase64PrivateKeyToTemporaryFile(t *testing.T) {
	keyContent := "-----BEGIN OPENSSH PRIVATE KEY-----\ntest\n-----END OPENSSH PRIVATE KEY-----\n"
	opts, cleanup, err := PrepareSSHOptions(SSHOptions{
		PrivateKeyBase64: base64.StdEncoding.EncodeToString([]byte(keyContent)),
	})
	if err != nil {
		t.Fatalf("PrepareSSHOptions returned error: %v", err)
	}
	defer cleanup()

	if opts.KeyPath == "" {
		t.Fatal("expected temporary key path")
	}
	if opts.PrivateKeyBase64 != "" {
		t.Fatal("expected private key base64 to be cleared after materialization")
	}
	got, err := os.ReadFile(opts.KeyPath)
	if err != nil {
		t.Fatalf("read temporary key: %v", err)
	}
	if string(got) != keyContent {
		t.Fatalf("expected key content %q, got %q", keyContent, string(got))
	}

	cleanup()
	if _, err := os.Stat(opts.KeyPath); !os.IsNotExist(err) {
		t.Fatalf("expected cleanup to remove temporary key, stat err: %v", err)
	}
}

func TestPrepareSSHOptionsRejectsMultipleKeySources(t *testing.T) {
	_, _, err := PrepareSSHOptions(SSHOptions{
		KeyPath:       "/tmp/key",
		PrivateKeyPEM: "key",
	})
	if err == nil {
		t.Fatal("expected error for multiple key sources")
	}
}
