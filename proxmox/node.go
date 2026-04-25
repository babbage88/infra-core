package proxmox

type ProxmoxVmId int
type ProxmoxVmName string

type ProxmoxNode struct {
	Hostname      string               `json:"hostNode"`
	PvePort       int                  `json:"pvePort"`
	QemuVMs       []QemuVm             `json:"qemuVMs,omitempty"`
	LxcContainers []LxcContainer       `json:"lxcContainers,omitempty"`
	Storage       []ProxmoxStoragePool `json:"storage,omitempty"`
}
