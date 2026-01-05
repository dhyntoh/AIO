package installer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Config struct {
	AdminID int64
	Domain  string
}

func RunSetup(ctx context.Context, cfg Config) error {
	steps := []struct {
		name string
		fn   func(context.Context, Config) error
	}{
		{"apt_update", runAptUpdate},
		{"install_dependencies", installDependencies},
		{"install_golang", installGolang},
		{"request_ssl", requestSSL},
		{"enable_bbr", enableBBR},
		{"install_xray", installXray},
		{"install_hysteria", installHysteria},
		{"install_zivpn", installZiVPN},
		{"configure_fail2ban", configureFail2Ban},
	}

	for _, step := range steps {
		if err := step.fn(ctx, cfg); err != nil {
			return fmt.Errorf("step %s failed: %w", step.name, err)
		}
	}
	return nil
}

func runAptUpdate(ctx context.Context, _ Config) error {
	return runCommand(ctx, "apt-get", "update")
}

func installDependencies(ctx context.Context, _ Config) error {
	packages := []string{"certbot", "socat", "cron", "fail2ban", "vnstat", "unzip", "openssl", "ufw"}
	args := append([]string{"install", "-y"}, packages...)
	return runCommand(ctx, "apt-get", args...)
}

func installGolang(ctx context.Context, _ Config) error {
	if _, err := exec.LookPath("go"); err == nil {
		return nil
	}

	version := "1.22.4"
	tarball := fmt.Sprintf("go%s.linux-amd64.tar.gz", version)
	url := fmt.Sprintf("https://go.dev/dl/%s", tarball)
	if err := downloadFile(ctx, url, "/tmp/"+tarball); err != nil {
		return err
	}
	if err := runCommand(ctx, "rm", "-rf", "/usr/local/go"); err != nil {
		return err
	}
	if err := runCommand(ctx, "tar", "-C", "/usr/local", "-xzf", "/tmp/"+tarball); err != nil {
		return err
	}

	profile := "/etc/profile.d/go.sh"
	content := "export PATH=$PATH:/usr/local/go/bin\n"
	return os.WriteFile(profile, []byte(content), 0o644)
}

func requestSSL(ctx context.Context, cfg Config) error {
	if cfg.Domain == "" {
		return errors.New("domain is required for SSL")
	}
	mail := fmt.Sprintf("admin@%s", cfg.Domain)
	return runCommand(ctx, "certbot", "certonly", "--standalone", "-d", cfg.Domain, "--non-interactive", "--agree-tos", "-m", mail)
}

func enableBBR(ctx context.Context, _ Config) error {
	config := "net.core.default_qdisc=fq\nnet.ipv4.tcp_congestion_control=bbr\n"
	path := "/etc/sysctl.d/99-tunnelzero.conf"
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		return err
	}
	return runCommand(ctx, "sysctl", "--system")
}

func installXray(ctx context.Context, _ Config) error {
	url := "https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-64.zip"
	zipPath := "/tmp/xray.zip"
	if err := downloadFile(ctx, url, zipPath); err != nil {
		return err
	}
	if err := runCommand(ctx, "unzip", "-o", zipPath, "-d", "/usr/local/bin"); err != nil {
		return err
	}
	return nil
}

func installHysteria(ctx context.Context, _ Config) error {
	url := "https://github.com/apernet/hysteria/releases/latest/download/hysteria-linux-amd64"
	path := "/usr/local/bin/hysteria"
	if err := downloadFile(ctx, url, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o755)
}

func installZiVPN(ctx context.Context, _ Config) error {
	fmt.Println("Installing ZiVPN UDP module...")
	if err := runCommand(ctx, "apt-get", "upgrade", "-y"); err != nil {
		return err
	}
	_ = runCommand(ctx, "systemctl", "stop", "zivpn.service")

	if err := downloadFile(ctx, "https://github.com/zahidbd2/udp-zivpn/releases/download/udp-zivpn_1.4.9/udp-zivpn-linux-amd64", "/usr/local/bin/zivpn"); err != nil {
		return err
	}
	if err := os.Chmod("/usr/local/bin/zivpn", 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll("/etc/zivpn", 0o755); err != nil {
		return err
	}
	if err := downloadFile(ctx, "https://raw.githubusercontent.com/zahidbd2/udp-zivpn/main/config.json", "/etc/zivpn/config.json"); err != nil {
		return err
	}

	if err := runCommand(ctx, "openssl", "req", "-new", "-newkey", "rsa:4096", "-days", "365", "-nodes", "-x509", "-subj", "/C=US/ST=California/L=Los Angeles/O=Example Corp/OU=IT Department/CN=zivpn", "-keyout", "/etc/zivpn/zivpn.key", "-out", "/etc/zivpn/zivpn.crt"); err != nil {
		return err
	}

	if err := runCommand(ctx, "sysctl", "-w", "net.core.rmem_max=16777216"); err != nil {
		return err
	}
	if err := runCommand(ctx, "sysctl", "-w", "net.core.wmem_max=16777216"); err != nil {
		return err
	}

	service := `[Unit]
Description=zivpn VPN Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/etc/zivpn
ExecStart=/usr/local/bin/zivpn server -c /etc/zivpn/config.json
Restart=always
RestartSec=3
Environment=ZIVPN_LOG_LEVEL=info
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
`
	if err := os.WriteFile("/etc/systemd/system/zivpn.service", []byte(service), 0o644); err != nil {
		return err
	}

	if err := updateZiVPNPasswords("/etc/zivpn/config.json"); err != nil {
		return err
	}

	if err := runCommand(ctx, "systemctl", "enable", "zivpn.service"); err != nil {
		return err
	}
	if err := runCommand(ctx, "systemctl", "start", "zivpn.service"); err != nil {
		return err
	}
	if err := runCommand(ctx, "bash", "-c", "iptables -t nat -A PREROUTING -i $(ip -4 route ls|grep default|grep -Po '(?<=dev )(\\S+)'|head -1) -p udp --dport 6000:19999 -j DNAT --to-destination :5667"); err != nil {
		return err
	}
	_ = runCommand(ctx, "ufw", "allow", "6000:19999/udp")
	_ = runCommand(ctx, "ufw", "allow", "5667/udp")

	fmt.Println("ZiVPN UDP Installed")
	return nil
}

func updateZiVPNPasswords(path string) error {
	type config struct {
		Config []string `json:"config"`
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cfg config
	if err := json.Unmarshal(content, &cfg); err != nil {
		return err
	}
	passwords := parseZiVPNPasswords(os.Getenv("ZIVPN_PASSWORDS"))
	cfg.Config = passwords
	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, updated, 0o644)
}

func parseZiVPNPasswords(input string) []string {
	if strings.TrimSpace(input) == "" {
		return []string{"zi"}
	}
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 1 {
		out = append(out, out[0])
	}
	if len(out) == 0 {
		return []string{"zi"}
	}
	return out
}

func configureFail2Ban(_ context.Context, _ Config) error {
	filter := "[Definition]\nfailregex = Token invalid from IP: <HOST>\n"
	filterPath := "/etc/fail2ban/filter.d/tunnelzero.conf"
	if err := os.WriteFile(filterPath, []byte(filter), 0o644); err != nil {
		return err
	}

	jail := strings.Join([]string{
		"[tunnelzero]",
		"enabled = true",
		"filter = tunnelzero",
		"port = ssh",
		"logpath = /var/log/tunnelzero-auth.log",
		"maxretry = 3",
		"bantime = 3600",
		""}, "\n")
	jailPath := "/etc/fail2ban/jail.d/tunnelzero.local"
	if err := os.WriteFile(jailPath, []byte(jail), 0o644); err != nil {
		return err
	}
	return nil
}

func runCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func downloadFile(ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, resp.Body)
	return err
}
