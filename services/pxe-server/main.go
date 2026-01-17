package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PXE Provisioning Server
// Serves Linux installation files over HTTP and manages PXE boot configurations

var (
	port        = getEnv("PORT", "8080")
	tftpRoot    = getEnv("TFTP_ROOT", "/tftpboot")
	httpRoot    = getEnv("HTTP_ROOT", "/var/www/html")
	isoDir      = getEnv("ISO_DIR", "/iso")
	serverIP    = getEnv("SERVER_IP", "192.168.8.197")
	statusMu    sync.RWMutex
	serverStatus = &Status{
		State:   "initializing",
		Message: "PXE server starting up...",
	}
)

type Status struct {
	State       string    `json:"state"`
	Message     string    `json:"message"`
	LastUpdated time.Time `json:"lastUpdated"`
	ISOs        []ISOInfo `json:"isos,omitempty"`
	Clients     []Client  `json:"clients,omitempty"`
}

type ISOInfo struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Ready    bool   `json:"ready"`
	Distro   string `json:"distro"`
	Version  string `json:"version"`
}

type Client struct {
	MAC       string    `json:"mac"`
	IP        string    `json:"ip"`
	Hostname  string    `json:"hostname"`
	LastSeen  time.Time `json:"lastSeen"`
	Status    string    `json:"status"`
}

type ProvisionRequest struct {
	Distro   string `json:"distro"`
	Version  string `json:"version"`
	Hostname string `json:"hostname"`
	Username string `json:"username"`
	Password string `json:"password"`
	SSHKey   string `json:"sshKey,omitempty"`
	Disk     string `json:"disk"`
	Timezone string `json:"timezone"`
	Locale   string `json:"locale"`
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func updateStatus(state, message string) {
	statusMu.Lock()
	defer statusMu.Unlock()
	serverStatus.State = state
	serverStatus.Message = message
	serverStatus.LastUpdated = time.Now()
}

func listISOs() []ISOInfo {
	var isos []ISOInfo
	entries, err := os.ReadDir(isoDir)
	if err != nil {
		return isos
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".iso") {
			continue
		}
		info, _ := entry.Info()
		iso := ISOInfo{
			Name:  entry.Name(),
			Size:  info.Size(),
			Ready: true,
		}
		// Parse distro info from filename
		name := strings.ToLower(entry.Name())
		if strings.Contains(name, "ubuntu") {
			iso.Distro = "ubuntu"
		} else if strings.Contains(name, "debian") {
			iso.Distro = "debian"
		} else if strings.Contains(name, "rocky") {
			iso.Distro = "rocky"
		} else if strings.Contains(name, "alma") {
			iso.Distro = "almalinux"
		}
		isos = append(isos, iso)
	}
	return isos
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "pxe-server"})
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	statusMu.RLock()
	status := *serverStatus
	statusMu.RUnlock()

	status.ISOs = listISOs()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func downloadISO(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL    string `json:"url"`
		Name   string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	go func() {
		updateStatus("downloading", fmt.Sprintf("Downloading %s...", req.Name))

		destPath := filepath.Join(isoDir, req.Name)
		out, err := os.Create(destPath)
		if err != nil {
			updateStatus("error", fmt.Sprintf("Failed to create file: %v", err))
			return
		}
		defer out.Close()

		resp, err := http.Get(req.URL)
		if err != nil {
			updateStatus("error", fmt.Sprintf("Failed to download: %v", err))
			return
		}
		defer resp.Body.Close()

		_, err = io.Copy(out, resp.Body)
		if err != nil {
			updateStatus("error", fmt.Sprintf("Failed to save: %v", err))
			return
		}

		updateStatus("ready", fmt.Sprintf("Downloaded %s successfully", req.Name))

		// Extract ISO for netboot
		extractISO(destPath)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "downloading"})
}

func extractISO(isoPath string) {
	isoName := filepath.Base(isoPath)
	extractDir := filepath.Join(httpRoot, strings.TrimSuffix(isoName, ".iso"))

	os.MkdirAll(extractDir, 0755)

	// Mount and copy ISO contents
	mountPoint := "/mnt/iso"
	os.MkdirAll(mountPoint, 0755)

	exec.Command("mount", "-o", "loop", isoPath, mountPoint).Run()
	exec.Command("cp", "-r", mountPoint+"/.", extractDir).Run()
	exec.Command("umount", mountPoint).Run()

	// Copy kernel and initrd to TFTP root
	copyNetbootFiles(extractDir, isoName)
}

func copyNetbootFiles(extractDir, isoName string) {
	distroDir := filepath.Join(tftpRoot, strings.TrimSuffix(isoName, ".iso"))
	os.MkdirAll(distroDir, 0755)

	// Ubuntu/Debian paths
	kernelPaths := []string{
		filepath.Join(extractDir, "casper/vmlinuz"),
		filepath.Join(extractDir, "install/vmlinuz"),
		filepath.Join(extractDir, "images/pxeboot/vmlinuz"),
	}
	initrdPaths := []string{
		filepath.Join(extractDir, "casper/initrd"),
		filepath.Join(extractDir, "install/initrd.gz"),
		filepath.Join(extractDir, "images/pxeboot/initrd.img"),
	}

	for _, kp := range kernelPaths {
		if _, err := os.Stat(kp); err == nil {
			exec.Command("cp", kp, filepath.Join(distroDir, "vmlinuz")).Run()
			break
		}
	}
	for _, ip := range initrdPaths {
		if _, err := os.Stat(ip); err == nil {
			exec.Command("cp", ip, filepath.Join(distroDir, "initrd")).Run()
			break
		}
	}
}

func generatePXEConfig(w http.ResponseWriter, r *http.Request) {
	var req ProvisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Generate autoinstall/preseed based on distro
	var config string
	switch req.Distro {
	case "ubuntu":
		config = generateUbuntuAutoinstall(req)
	case "debian":
		config = generateDebianPreseed(req)
	default:
		config = generateUbuntuAutoinstall(req)
	}

	// Save config
	configPath := filepath.Join(httpRoot, "autoinstall", req.Hostname+".yaml")
	os.MkdirAll(filepath.Dir(configPath), 0755)
	os.WriteFile(configPath, []byte(config), 0644)

	// Generate PXE boot entry
	pxeConfig := generatePXEBootEntry(req)
	pxePath := filepath.Join(tftpRoot, "pxelinux.cfg", "01-"+strings.ReplaceAll(req.Hostname, ":", "-"))
	os.MkdirAll(filepath.Dir(pxePath), 0755)
	os.WriteFile(pxePath, []byte(pxeConfig), 0644)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":     "configured",
		"configPath": configPath,
		"pxePath":    pxePath,
	})
}

func generateUbuntuAutoinstall(req ProvisionRequest) string {
	timezone := req.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	locale := req.Locale
	if locale == "" {
		locale = "en_US.UTF-8"
	}
	disk := req.Disk
	if disk == "" {
		disk = "/dev/sda"
	}
	username := req.Username
	if username == "" {
		username = "admin"
	}
	password := req.Password
	if password == "" {
		password = "19209746"
	}
	hostname := req.Hostname
	if hostname == "" {
		hostname = "linux-server"
	}

	config := fmt.Sprintf(`#cloud-config
autoinstall:
  version: 1
  locale: %s
  keyboard:
    layout: us
  identity:
    hostname: %s
    username: %s
    password: %s
  ssh:
    install-server: true
    authorized-keys:
      - %s
    allow-pw: true
  storage:
    layout:
      name: lvm
      match:
        path: %s
  packages:
    - openssh-server
    - curl
    - vim
    - htop
    - net-tools
  user-data:
    disable_root: false
    chpasswd:
      expire: false
      list:
        - root:%s
  late-commands:
    - curtin in-target --target=/target -- systemctl enable ssh
    - curtin in-target --target=/target -- sed -i 's/^#PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config
    - curtin in-target --target=/target -- sed -i 's/^PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config
`, locale, hostname, username, password, req.SSHKey, disk, password)

	return config
}

func generateDebianPreseed(req ProvisionRequest) string {
	password := req.Password
	if password == "" {
		password = "19209746"
	}
	hostname := req.Hostname
	if hostname == "" {
		hostname = "linux-server"
	}
	username := req.Username
	if username == "" {
		username = "admin"
	}
	timezone := req.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	disk := req.Disk
	if disk == "" {
		disk = "/dev/sda"
	}

	return fmt.Sprintf(`# Debian Preseed
d-i debian-installer/locale string en_US.UTF-8
d-i keyboard-configuration/xkb-keymap select us
d-i netcfg/choose_interface select auto
d-i netcfg/get_hostname string %s
d-i netcfg/get_domain string local
d-i passwd/root-login boolean true
d-i passwd/root-password password %s
d-i passwd/root-password-again password %s
d-i passwd/make-user boolean true
d-i passwd/username string %s
d-i passwd/user-fullname string %s
d-i passwd/user-password password %s
d-i passwd/user-password-again password %s
d-i clock-setup/utc boolean true
d-i time/zone string %s
d-i partman-auto/method string lvm
d-i partman-auto/disk string %s
d-i partman-lvm/confirm boolean true
d-i partman-lvm/confirm_nooverwrite boolean true
d-i partman/confirm_write_new_label boolean true
d-i partman/choose_partition select finish
d-i partman/confirm boolean true
d-i partman/confirm_nooverwrite boolean true
d-i apt-setup/use_mirror boolean true
tasksel tasksel/first multiselect standard, ssh-server
d-i pkgsel/include string openssh-server curl vim htop
d-i grub-installer/only_debian boolean true
d-i grub-installer/bootdev string %s
d-i preseed/late_command string \
    in-target sed -i 's/^#PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config; \
    in-target sed -i 's/^PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config;
d-i finish-install/reboot_in_progress note
`, hostname, password, password, username, username, password, password,
		timezone, disk, disk)
}

func generatePXEBootEntry(req ProvisionRequest) string {
	return fmt.Sprintf(`DEFAULT install
LABEL install
    KERNEL %s/vmlinuz
    INITRD %s/initrd
    APPEND ip=dhcp url=http://%s/autoinstall/%s.yaml autoinstall ds=nocloud-net;s=http://%s/autoinstall/ ---
`, req.Distro, req.Distro, serverIP, req.Hostname, serverIP)
}

func listDistrosHandler(w http.ResponseWriter, r *http.Request) {
	distros := []map[string]string{
		{"name": "Ubuntu 22.04 LTS", "id": "ubuntu-22.04", "url": "https://releases.ubuntu.com/22.04/ubuntu-22.04.3-live-server-amd64.iso"},
		{"name": "Ubuntu 24.04 LTS", "id": "ubuntu-24.04", "url": "https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso"},
		{"name": "Debian 12", "id": "debian-12", "url": "https://cdimage.debian.org/debian-cd/current/amd64/iso-cd/debian-12.4.0-amd64-netinst.iso"},
		{"name": "Rocky Linux 9", "id": "rocky-9", "url": "https://download.rockylinux.org/pub/rocky/9/isos/x86_64/Rocky-9.3-x86_64-minimal.iso"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(distros)
}

func setupDefaultPXEMenu() {
	// Create default PXE menu
	pxeMenu := fmt.Sprintf(`UI menu.c32
PROMPT 0
TIMEOUT 300
MENU TITLE HolmOS PXE Boot Menu

LABEL ubuntu
    MENU LABEL Install Ubuntu 22.04 LTS
    KERNEL ubuntu-22.04/vmlinuz
    INITRD ubuntu-22.04/initrd
    APPEND ip=dhcp url=http://%s/ubuntu-22.04/ autoinstall ds=nocloud-net;s=http://%s/autoinstall/ ---

LABEL debian
    MENU LABEL Install Debian 12
    KERNEL debian-12/vmlinuz
    INITRD debian-12/initrd
    APPEND ip=dhcp url=http://%s/debian-12/ priority=critical auto=true preseed/url=http://%s/preseed.cfg ---

LABEL local
    MENU LABEL Boot from local disk
    LOCALBOOT 0
`, serverIP, serverIP, serverIP, serverIP)

	os.MkdirAll(filepath.Join(tftpRoot, "pxelinux.cfg"), 0755)
	os.WriteFile(filepath.Join(tftpRoot, "pxelinux.cfg", "default"), []byte(pxeMenu), 0644)
}

func main() {
	log.Println(`
    ╔═══════════════════════════════════════════════════════════════════╗
    ║              PXE PROVISIONING SERVER v1.0                          ║
    ║           Network Boot Linux Installation Service                  ║
    ╠═══════════════════════════════════════════════════════════════════╣
    ║  • PXE/TFTP boot server for x86 Linux installation                 ║
    ║  • Supports Ubuntu, Debian, Rocky Linux, AlmaLinux                 ║
    ║  • Automated installation via autoinstall/preseed                  ║
    ║  • HTTP server for installation files                              ║
    ╚═══════════════════════════════════════════════════════════════════╝
	`)

	// Create directories
	os.MkdirAll(tftpRoot, 0755)
	os.MkdirAll(httpRoot, 0755)
	os.MkdirAll(isoDir, 0755)
	os.MkdirAll(filepath.Join(httpRoot, "autoinstall"), 0755)

	// Setup default PXE menu
	setupDefaultPXEMenu()

	updateStatus("ready", "PXE server is ready")

	// API routes
	http.HandleFunc("/", statusHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/api/status", statusHandler)
	http.HandleFunc("/api/distros", listDistrosHandler)
	http.HandleFunc("/api/download", downloadISO)
	http.HandleFunc("/api/provision", generatePXEConfig)
	http.HandleFunc("/api/isos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listISOs())
	})

	// Serve static files for installation
	http.Handle("/files/", http.StripPrefix("/files/", http.FileServer(http.Dir(httpRoot))))

	log.Printf("PXE Server listening on port %s", port)
	log.Printf("TFTP root: %s", tftpRoot)
	log.Printf("HTTP root: %s", httpRoot)
	log.Printf("Server IP: %s", serverIP)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
