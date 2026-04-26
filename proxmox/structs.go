package proxmox

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
)

// QemuVm represents basic information about a VM from Proxmox.
type QemuVm struct {
	Vmid              int     `json:"vmid"`
	Name              string  `json:"name"`
	Status            string  `json:"status"`
	CPU               float64 `json:"cpu,omitempty"`
	MaxMem            int64   `json:"maxmem,omitempty"`
	MemHost           int     `json:"memhost,omitempty"`
	Mem               int64   `json:"mem,omitempty"`
	MaxDisk           int64   `json:"maxdisk,omitempty"`
	NetIn             int     `json:"netin,omitempty"`
	NetOut            int     `json:"netout,omitempty"`
	Disk              int64   `json:"disk,omitempty"`
	DiskRead          int     `json:"diskread,omitempty"`
	DiskWrite         int     `json:"diskwrite,omitempty"`
	Node              string  `json:"node,omitempty"`
	PID               int     `json:"pid,omitempty"`
	PresureCpuFull    float64 `json:"pressurecpufull,omitempty"`
	PresureCpuSome    float64 `json:"pressurecpusome,omitempty"`
	PresureIoFull     float64 `json:"pressureiofull,omitempty"`
	PresureIoSome     float64 `json:"pressureiosome,omitempty"`
	PresureMemoryFull float64 `json:"pressurememoryfull,omitempty"`
	PresureMemorySome float64 `json:"pressurememorysome,omitempty"`
	QmStatus          string  `json:"qmstatus,omitempty"`
	RunningMachine    string  `json:"running-machine,omitempty"`
	RunningQemu       string  `json:"running-qemu,omitempty"`
	Serial            int     `json:"serial,omitempty"`
	Tags              string  `json:"tags,omitempty"`
	Template          int     `json:"template,omitempty"`
	Uptime            int64   `json:"uptime,omitempty"`
}

func ParseQemuVmConfig(raw map[string]any) *ProxmoxQemuVmConfig {
	cfg := &ProxmoxQemuVmConfig{Raw: make(map[string]string)}
	for k, v := range raw {
		switch k {
		case "name":
			cfg.Name = fmt.Sprintf("%v", v)
		case "memory":
			cfg.MemoryMB = toJSONNumber(v)
		case "sockets":
			cfg.Sockets = toJSONNumber(v)
		case "cores":
			cfg.Cores = toJSONNumber(v)
		case "description":
			cfg.Description = fmt.Sprintf("%v", v)
		default:
			cfg.Raw[k] = fmt.Sprintf("%v", v)
		}
	}
	return cfg
}

// ProxmoxQemuVmConfig represents common VM configuration fields.
type ProxmoxQemuVmConfig struct {
	Name        string      `json:"name,omitempty"`
	Vmid        json.Number `json:"vmid,omitempty"`
	MemoryMB    json.Number `json:"memory,omitempty"`
	Sockets     json.Number `json:"sockets,omitempty"`
	Cores       json.Number `json:"cores,omitempty"`
	Description string      `json:"description,omitempty"`
	// Raw holds additional fields not mapped above.
	Raw map[string]string
}

// Auth stores the Proxmox API token-based credentials.
type Auth struct {
	Host     string // e.g. "https://proxmox.example.com:8006"
	ApiToken string // Format: "USER@REALM!TOKENID=SECRET"
}

type LxcContainer struct {
	Node          string   `json:"node,omitempty"` // Used in URL, not payload
	VmId          int      `json:"vmid,omitempty"` // Required
	Name          string   `json:"name,omitempty"`
	Hostname      string   `json:"hostname,omitempty"` // Required
	Status        string   `json:"status,omitempty"`
	CPU           float64  `json:"cpu,omitempty"`
	MaxMem        int64    `json:"maxmem,omitempty"`
	Mem           int64    `json:"mem,omitempty"`
	MaxDisk       int64    `json:"maxdisk,omitempty"`
	Disk          int64    `json:"disk,omitempty"`
	Password      string   `json:"password,omitempty"`   // Required if not using SSH key
	OsTemplate    string   `json:"ostemplate,omitempty"` // Required (e.g., "local:vztmpl/ubuntu-22.04-standard_22.04-1_amd64.tar.zst")
	Storage       string   `json:"storage,omitempty"`    // Required (storage ID for rootfs)
	RootFsSize    string   `json:"rootfs,omitempty"`     // Required (e.g., "8G")
	Memory        int      `json:"memory,omitempty"`     // RAM in MB
	Swap          int      `json:"swap,omitempty"`       // Swap in MB
	Cores         int      `json:"cores,omitempty"`      // CPU cores
	CpuLimit      int      `json:"cpulimit,omitempty"`   // Limit in % of total
	CpuUnits      int      `json:"cpuunits,omitempty"`   // Relative CPU weight
	Net0          string   `json:"net0,omitempty"`       // Network config string
	Bridge        string   `json:"bridge,omitempty"`     // Optional, if setting bridge separately
	Nameserver    string   `json:"nameserver,omitempty"` // DNS
	Searchdomain  string   `json:"searchdomain,omitempty"`
	Pool          string   `json:"pool,omitempty"` // Optional pool
	Description   string   `json:"description,omitempty"`
	Unprivileged  string   `json:"unprivileged,omitempty"` // 1 or 0
	Start         string   `json:"start,omitempty"`        // 1 to auto-start
	BwLimit       int      `json:"bwlimit,omitempty"`
	Arch          string   `json:"arch,omitempty"`     // e.g., "amd64"
	Cmode         string   `json:"cmode,omitempty"`    // e.g., "tty"
	Console       string   `json:"console,omitempty"`  // 0 or 1
	Debug         int      `json:"debug,omitempty"`    // 0 or 1
	Features      string   `json:"features,omitempty"` // Comma-separated list
	Startup       string   `json:"startup,omitempty"`  // Startup order string
	Tags          string   `json:"tags,omitempty"`     // Comma-separated tags
	Template      int      `json:"template,omitempty"`
	Uptime        int64    `json:"uptime,omitempty"`
	SshPublicKeys []string `json:"ssh_public_keys,omitempty"` // SSH keys string
}

func (lxc *LxcContainer) ToFormParams() map[string]string {
	params := make(map[string]string)

	if lxc.VmId != 0 {
		params["vmid"] = fmt.Sprintf("%d", lxc.VmId)
	}
	if lxc.Hostname != "" {
		params["hostname"] = lxc.Hostname
	}
	if lxc.Password != "" {
		params["password"] = lxc.Password
	}
	if lxc.OsTemplate != "" {
		params["ostemplate"] = lxc.OsTemplate
	}
	if len(lxc.SshPublicKeys) >= 1 {
		keysString, err := lxc.ParseSshPublicKeySlice()
		if err != nil {
			slog.Error("Error parsing ssh-public-keys", "error", err.Error(), slog.Any("keys", lxc.SshPublicKeys))
			os.Exit(1)
		}
		params["ssh-public-keys"] = keysString
	}
	if lxc.Storage != "" {
		params["storage"] = lxc.Storage
	}
	if lxc.RootFsSize != "" && lxc.Storage != "" {
		params["rootfs"] = fmt.Sprintf("%s:%s", lxc.Storage, lxc.RootFsSize)
	}
	if lxc.Memory != 0 {
		params["memory"] = fmt.Sprintf("%d", lxc.Memory)
	}
	if lxc.Swap != 0 {
		params["swap"] = fmt.Sprintf("%d", lxc.Swap)
	}
	if lxc.Cores != 0 {
		params["cores"] = fmt.Sprintf("%d", lxc.Cores)
	}
	if lxc.CpuLimit != 0 {
		params["cpulimit"] = fmt.Sprintf("%d", lxc.CpuLimit)
	}
	if lxc.CpuUnits != 0 {
		params["cpuunits"] = fmt.Sprintf("%d", lxc.CpuUnits)
	}
	if lxc.Net0 != "" {
		params["net0"] = lxc.Net0
	}
	if lxc.Arch != "" {
		params["arch"] = lxc.Arch
	}
	if lxc.Cmode != "" {
		params["cmode"] = lxc.Cmode
	}
	if lxc.Start != "" {
		params["start"] = lxc.Start
	}
	if lxc.Console != "" {
		params["console"] = lxc.Console
	}
	if lxc.Unprivileged != "" {
		params["unprivileged"] = lxc.Unprivileged
	}
	if lxc.Features != "" {
		params["features"] = lxc.Features
	}

	return params
}
