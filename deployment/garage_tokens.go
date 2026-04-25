package deployment

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type GarageTokenInternalRequest struct {
	BucketName         string
	KeyName            string
	CreateBucket       bool
	AllowCreateBuckets bool
	AllowRead          bool
	AllowWrite         bool
	AllowOwner         bool
	BinaryPath         string
	ConfigPath         string
	LayoutZone         string
	LayoutCapacity     string
}

type GarageS3Credentials struct {
	BucketName      string
	KeyName         string
	AccessKeyID     string
	SecretAccessKey string
}

func (rgi *RemoteGarageInstaller) CreateS3Token(req GarageTokenInternalRequest) (*GarageS3Credentials, error) {
	if strings.TrimSpace(req.KeyName) == "" {
		return nil, fmt.Errorf("garage key name is required")
	}
	if strings.TrimSpace(req.BinaryPath) == "" {
		req.BinaryPath = "/usr/local/bin/garage"
	}
	if strings.TrimSpace(req.ConfigPath) == "" {
		req.ConfigPath = "/etc/garage.toml"
	}
	if strings.TrimSpace(req.LayoutZone) == "" {
		req.LayoutZone = "dc1"
	}
	if strings.TrimSpace(req.LayoutCapacity) == "" {
		req.LayoutCapacity = "100G"
	}
	requiresBucket := strings.TrimSpace(req.BucketName) != "" || req.CreateBucket || req.AllowRead || req.AllowWrite || req.AllowOwner
	if requiresBucket && strings.TrimSpace(req.BucketName) == "" {
		return nil, fmt.Errorf("garage bucket name is required when bucket access flags are used")
	}
	if req.CreateBucket {
		if err := rgi.ensureBucketExists(req.BinaryPath, req.ConfigPath, req.BucketName); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "layout not ready") {
				if layoutErr := rgi.ensureSingleNodeLayout(req.BinaryPath, req.ConfigPath, req.LayoutZone, req.LayoutCapacity); layoutErr != nil {
					return nil, layoutErr
				}
				if retryErr := rgi.ensureBucketExists(req.BinaryPath, req.ConfigPath, req.BucketName); retryErr != nil {
					return nil, retryErr
				}
			} else {
				return nil, err
			}
		}
	}
	keyInfo, err := rgi.ensureKeyExists(req.BinaryPath, req.ConfigPath, req.KeyName)
	if err != nil {
		return nil, err
	}
	if req.AllowCreateBuckets {
		if err := rgi.allowKeyToCreateBuckets(req.BinaryPath, req.ConfigPath, req.KeyName); err != nil {
			return nil, err
		}
	}
	if requiresBucket {
		if err := rgi.allowBucketForKey(req.BinaryPath, req.ConfigPath, req.BucketName, req.KeyName, req.AllowRead, req.AllowWrite, req.AllowOwner); err != nil {
			return nil, err
		}
	}
	return &GarageS3Credentials{
		BucketName:      req.BucketName,
		KeyName:         req.KeyName,
		AccessKeyID:     keyInfo.AccessKeyID,
		SecretAccessKey: keyInfo.SecretAccessKey,
	}, nil
}

func (rgi *RemoteGarageInstaller) ensureBucketExists(binaryPath, configPath, bucketName string) error {
	cmd := shellJoinWords([]string{binaryPath, "-c", configPath, "bucket", "create", bucketName})
	out, err := rgi.SshClient.Run("sh -c " + ShellQuote(cmd))
	if err != nil {
		output := strings.TrimSpace(string(out))
		if strings.Contains(strings.ToLower(output), "already exists") {
			return nil
		}
		return formatRemoteCommandError(fmt.Errorf("create garage bucket: %w", err), out)
	}
	return nil
}

func (rgi *RemoteGarageInstaller) ensureKeyExists(binaryPath, configPath, keyName string) (*GarageS3Credentials, error) {
	createCmd := shellJoinWords([]string{binaryPath, "-c", configPath, "key", "create", keyName})
	out, err := rgi.SshClient.Run("sh -c " + ShellQuote(createCmd))
	if err != nil {
		output := strings.TrimSpace(string(out))
		if !strings.Contains(strings.ToLower(output), "already exists") {
			return nil, formatRemoteCommandError(fmt.Errorf("create garage key: %w", err), out)
		}
	}
	infoCmd := shellJoinWords([]string{binaryPath, "-c", configPath, "key", "info", keyName, "--show-secret"})
	out, err = rgi.SshClient.Run("sh -c " + ShellQuote(infoCmd))
	if err != nil {
		return nil, formatRemoteCommandError(fmt.Errorf("inspect garage key: %w", err), out)
	}
	creds, parseErr := parseGarageKeyInfoOutput(string(out))
	if parseErr != nil {
		return nil, parseErr
	}
	creds.KeyName = keyName
	return creds, nil
}

func (rgi *RemoteGarageInstaller) allowKeyToCreateBuckets(binaryPath, configPath, keyName string) error {
	cmd := shellJoinWords([]string{binaryPath, "-c", configPath, "key", "allow", "--create-bucket", keyName})
	out, err := rgi.SshClient.Run("sh -c " + ShellQuote(cmd))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("grant garage key create-bucket permission: %w", err), out)
	}
	return nil
}

func (rgi *RemoteGarageInstaller) allowBucketForKey(binaryPath, configPath, bucketName, keyName string, allowRead, allowWrite, allowOwner bool) error {
	args := []string{binaryPath, "-c", configPath, "bucket", "allow"}
	if allowRead {
		args = append(args, "--read")
	}
	if allowWrite {
		args = append(args, "--write")
	}
	if allowOwner {
		args = append(args, "--owner")
	}
	args = append(args, bucketName, "--key", keyName)
	cmd := shellJoinWords(args)
	out, err := rgi.SshClient.Run("sh -c " + ShellQuote(cmd))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("grant garage bucket permissions: %w", err), out)
	}
	return nil
}

func parseGarageKeyInfoOutput(output string) (*GarageS3Credentials, error) {
	accessKeyRE := regexp.MustCompile(`(?m)^Key ID:\s*(\S+)\s*$`)
	secretKeyRE := regexp.MustCompile(`(?m)^Secret key:\s*(\S+)\s*$`)
	accessKeyMatch := accessKeyRE.FindStringSubmatch(output)
	secretKeyMatch := secretKeyRE.FindStringSubmatch(output)
	if len(accessKeyMatch) < 2 || len(secretKeyMatch) < 2 {
		return nil, fmt.Errorf("unable to parse Garage key info output")
	}
	return &GarageS3Credentials{
		AccessKeyID:     accessKeyMatch[1],
		SecretAccessKey: secretKeyMatch[1],
	}, nil
}

func (rgi *RemoteGarageInstaller) ensureSingleNodeLayout(binaryPath, configPath, zone, capacity string) error {
	statusCmd := shellJoinWords([]string{binaryPath, "-c", configPath, "status"})
	out, err := rgi.SshClient.Run("sh -c " + ShellQuote(statusCmd))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("inspect garage cluster status: %w", err), out)
	}
	nodeID, err := parseGarageNodeIDFromStatus(string(out))
	if err != nil {
		return err
	}
	assignCmd := shellJoinWords([]string{binaryPath, "-c", configPath, "layout", "assign", "-z", zone, "-c", capacity, nodeID})
	out, err = rgi.SshClient.Run("sh -c " + ShellQuote(assignCmd))
	if err != nil && !strings.Contains(strings.ToLower(string(out)), "staged") {
		return formatRemoteCommandError(fmt.Errorf("assign garage layout: %w", err), out)
	}
	showCmd := shellJoinWords([]string{binaryPath, "-c", configPath, "layout", "show"})
	out, err = rgi.SshClient.Run("sh -c " + ShellQuote(showCmd))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("inspect garage layout version: %w", err), out)
	}
	nextVersion, err := nextGarageLayoutVersion(string(out))
	if err != nil {
		return err
	}
	applyCmd := shellJoinWords([]string{binaryPath, "-c", configPath, "layout", "apply", "--version", strconv.Itoa(nextVersion)})
	out, err = rgi.SshClient.Run("sh -c " + ShellQuote(applyCmd))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("apply garage layout: %w", err), out)
	}
	return nil
}

func parseGarageNodeIDFromStatus(output string) (string, error) {
	nodeIDRE := regexp.MustCompile(`(?m)^([0-9a-f]{8,})\s+`)
	match := nodeIDRE.FindStringSubmatch(output)
	if len(match) < 2 {
		return "", fmt.Errorf("unable to determine Garage node ID from status output")
	}
	return match[1], nil
}

func nextGarageLayoutVersion(output string) (int, error) {
	versionRE := regexp.MustCompile(`(?m)^Current cluster layout version:\s*(\d+)\s*$`)
	match := versionRE.FindStringSubmatch(output)
	if len(match) < 2 {
		return 0, fmt.Errorf("unable to determine current Garage layout version")
	}
	currentVersion, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, fmt.Errorf("parse Garage layout version: %w", err)
	}
	return currentVersion + 1, nil
}
