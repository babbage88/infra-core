package proxmox

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

type ProxmoxStorageType string
type ProxmoxStorageContentType string
type ProxmoxStorageEnabledContent map[ProxmoxStorageContentType]bool

type ProxmoxResource interface {
	ProxmoxQemuVmConfig | LxcContainer
}

const (
	Directory           ProxmoxStorageType = "directory"
	LVM                 ProxmoxStorageType = "lvm"
	LvmThin             ProxmoxStorageType = "lvm-thin"
	BTRFS               ProxmoxStorageType = "btrfs"
	NFS                 ProxmoxStorageType = "nfs"
	SmbCifs             ProxmoxStorageType = "cifs"
	GlusterFs           ProxmoxStorageType = "glusterfs"
	CephFs              ProxmoxStorageType = "cephfs"
	RBD                 ProxmoxStorageType = "rbd"
	ZfsOverIscsi        ProxmoxStorageType = "zfs-iscsi"
	ZFS                 ProxmoxStorageType = "zfs"
	ProxmoxBackupServer ProxmoxStorageType = "pbs"
)

const (
	Backup            ProxmoxStorageContentType = "backup"
	Iso               ProxmoxStorageContentType = "iso"
	VmDiskImages      ProxmoxStorageContentType = "image"
	CloudInitSnippets ProxmoxStorageContentType = "snippets"
	LxcTemplates      ProxmoxStorageContentType = "vztmpl"
	ContainerRootDir  ProxmoxStorageContentType = "rootdir"
)

type ProxmoxStoragePool struct {
	Name         string                       `json:"name"`
	Type         ProxmoxStorageType           `json:"type"`
	Capabilities ProxmoxStorageEnabledContent `json:"capabilities"`
	Path         string                       `json:"path"`
	Server       string                       `json:"server"`
	Shared       bool                         `json:"shared"`
	TotalBytes   int                          `json:"totalBytes"`
	Used         int                          `json:"used"`
	Enabled      bool                         `json:"enabled"`
}

type NodeStorage struct {
	Storage string `json:"storage"`
	Type    string `json:"type,omitempty"`
	Content string `json:"content,omitempty"`
	Enabled int    `json:"enabled,omitempty"`
	Active  int    `json:"active,omitempty"`
	Shared  int    `json:"shared,omitempty"`
}

type StorageContentItem struct {
	Volid   string `json:"volid"`
	Content string `json:"content,omitempty"`
	Format  string `json:"format,omitempty"`
	Size    int64  `json:"size,omitempty"`
}

func (s NodeStorage) Supports(contentType ProxmoxStorageContentType) bool {
	if strings.TrimSpace(s.Content) == "" {
		return false
	}

	for _, part := range strings.Split(s.Content, ",") {
		if strings.TrimSpace(part) == string(contentType) {
			return true
		}
	}

	return false
}

func (c *Client) ListNodeStorage(ctx context.Context, node string) ([]NodeStorage, error) {
	path := fmt.Sprintf("%s/%s/storage", apiNodesPath, url.PathEscape(node))

	var storages []NodeStorage
	if err := c.do(ctx, "GET", path, nil, nil, false, &storages); err != nil {
		return nil, err
	}

	return storages, nil
}

func (c *Client) ListStorageContent(ctx context.Context, node, storage string, contentType ProxmoxStorageContentType) ([]StorageContentItem, error) {
	path := fmt.Sprintf("%s/%s/storage/%s/content", apiNodesPath, url.PathEscape(node), url.PathEscape(storage))

	var items []StorageContentItem
	if err := c.do(ctx, "GET", path, nil, nil, false, &items); err != nil {
		return nil, err
	}

	if contentType == "" {
		return items, nil
	}

	filtered := make([]StorageContentItem, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Content) != string(contentType) {
			continue
		}
		filtered = append(filtered, item)
	}

	return filtered, nil
}

func (c *Client) ListLxcTemplates(ctx context.Context, node string) ([]string, error) {
	storages, err := c.ListNodeStorage(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("list node storage: %w", err)
	}

	templates := make([]string, 0)
	seen := make(map[string]struct{})
	var skippedErrors []string

	for _, storage := range storages {
		if storage.Enabled == 0 || !storage.Supports(LxcTemplates) {
			continue
		}

		items, err := c.ListStorageContent(ctx, node, storage.Storage, LxcTemplates)
		if err != nil {
			if apiErr, ok := err.(*APIError); ok && (apiErr.Status == 501 || apiErr.Status == 403 || apiErr.Status == 400) {
				skippedErrors = append(skippedErrors, fmt.Sprintf("%s(status=%d)", storage.Storage, apiErr.Status))
				continue
			}
			return nil, fmt.Errorf("list templates for storage %q: %w", storage.Storage, err)
		}

		for _, item := range items {
			if strings.TrimSpace(item.Volid) == "" {
				continue
			}
			if _, ok := seen[item.Volid]; ok {
				continue
			}
			seen[item.Volid] = struct{}{}
			templates = append(templates, item.Volid)
		}
	}

	slices.Sort(templates)
	if len(templates) == 0 && len(skippedErrors) > 0 {
		return nil, fmt.Errorf("no templates found via API; skipped unsupported storages: %s", strings.Join(skippedErrors, ", "))
	}
	return templates, nil
}
