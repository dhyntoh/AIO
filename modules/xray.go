package modules

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"tunnelzero/models"

	"gorm.io/gorm"
)

type xrayInbound struct {
	Listen         string                 `json:"listen"`
	Port           int                    `json:"port"`
	Protocol       string                 `json:"protocol"`
	Settings       map[string]interface{} `json:"settings"`
	StreamSettings map[string]interface{} `json:"streamSettings"`
}

type xrayConfig struct {
	Log struct {
		Loglevel string `json:"loglevel"`
	} `json:"log"`
	Inbounds  []xrayInbound            `json:"inbounds"`
	Outbounds []map[string]interface{} `json:"outbounds"`
}

func RefreshXrayConfig(db *gorm.DB, domain string) error {
	var users []models.User
	if err := db.Where("protocol IN ?", []string{"vmess", "vless", "trojan"}).Find(&users).Error; err != nil {
		return err
	}
	config, err := BuildXrayConfig(users, domain)
	if err != nil {
		return err
	}
	return os.WriteFile("/etc/xray/config.json", config, 0o644)
}

func BuildXrayConfig(users []models.User, domain string) ([]byte, error) {
	vmessClients := make([]map[string]interface{}, 0)
	vlessClients := make([]map[string]interface{}, 0)
	trojanClients := make([]map[string]interface{}, 0)

	for _, user := range users {
		switch user.Protocol {
		case "vmess":
			vmessClients = append(vmessClients, map[string]interface{}{"id": user.UUID, "alterId": 0, "email": user.Username})
		case "vless":
			vlessClients = append(vlessClients, map[string]interface{}{"id": user.UUID, "email": user.Username, "flow": "xtls-rprx-vision"})
		case "trojan":
			trojanClients = append(trojanClients, map[string]interface{}{"password": user.Password, "email": user.Username})
		}
	}

	inbounds := []xrayInbound{
		{
			Listen:   "127.0.0.1",
			Port:     10000,
			Protocol: "vmess",
			Settings: map[string]interface{}{"clients": vmessClients},
			StreamSettings: map[string]interface{}{
				"network":  "tcp",
				"security": "none",
			},
		},
		{
			Listen:   "127.0.0.1",
			Port:     10001,
			Protocol: "vless",
			Settings: map[string]interface{}{"clients": vlessClients, "decryption": "none"},
			StreamSettings: map[string]interface{}{
				"network":  "tcp",
				"security": "reality",
				"realitySettings": map[string]interface{}{
					"show":        false,
					"dest":        fmt.Sprintf("%s:443", domain),
					"xver":        0,
					"serverNames": []string{domain},
					"privateKey":  "CHANGE_ME_PRIVATE_KEY",
					"shortIds":    []string{""},
				},
			},
		},
		{
			Listen:   "127.0.0.1",
			Port:     10002,
			Protocol: "trojan",
			Settings: map[string]interface{}{"clients": trojanClients},
			StreamSettings: map[string]interface{}{
				"network":  "tcp",
				"security": "tls",
				"tlsSettings": map[string]interface{}{
					"serverName": domain,
					"certificates": []map[string]string{{
						"certificateFile": fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", domain),
						"keyFile":         fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", domain),
					}},
				},
			},
		},
	}

	cfg := xrayConfig{
		Inbounds: inbounds,
		Outbounds: []map[string]interface{}{{
			"protocol": "freedom",
			"settings": map[string]interface{}{},
		}},
	}
	cfg.Log.Loglevel = "warning"

	return json.MarshalIndent(cfg, "", "  ")
}

func RemoveUser(db *gorm.DB, domain string, user models.User) error {
	if err := db.Delete(&user).Error; err != nil {
		return err
	}

	if user.Protocol == "hysteria" {
		if err := RefreshHysteriaConfig(db, domain); err != nil {
			return err
		}
		return nil
	}

	if err := RefreshXrayConfig(db, domain); err != nil {
		return err
	}

	return nil
}

func BuildVMessLink(user models.User, domain string) string {
	payload := map[string]interface{}{
		"v":    "2",
		"ps":   user.Username,
		"add":  domain,
		"port": "443",
		"id":   user.UUID,
		"aid":  "0",
		"net":  "tcp",
		"type": "none",
		"host": "",
		"path": "",
		"tls":  "tls",
	}
	encoded, _ := json.Marshal(payload)
	return fmt.Sprintf("vmess://%s", base64.StdEncoding.EncodeToString(encoded))
}

func BuildVLESSLink(user models.User, domain string) string {
	return fmt.Sprintf("vless://%s@%s:443?security=reality&type=tcp&flow=xtls-rprx-vision#%s", user.UUID, domain, user.Username)
}

func BuildTrojanLink(user models.User, domain string) string {
	return fmt.Sprintf("trojan://%s@%s:443?security=tls&type=tcp#%s", user.Password, domain, user.Username)
}
