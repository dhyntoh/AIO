package modules

import (
	"fmt"
	"os"

	"tunnelzero/models"

	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

type hysteriaConfig struct {
	Listen     string                 `yaml:"listen"`
	TLS        map[string]interface{} `yaml:"tls"`
	Auth       map[string]interface{} `yaml:"auth"`
	Bandwidth  map[string]interface{} `yaml:"bandwidth"`
	UserBundle []hysteriaUser         `yaml:"users"`
}

type hysteriaUser struct {
	Name     string `yaml:"name"`
	Password string `yaml:"password"`
}

func RefreshHysteriaConfig(db *gorm.DB, domain string) error {
	var users []models.User
	if err := db.Where("protocol = ?", "hysteria").Find(&users).Error; err != nil {
		return err
	}
	config, err := BuildHysteriaConfig(users, domain)
	if err != nil {
		return err
	}
	if err := os.MkdirAll("/etc/hysteria", 0o755); err != nil {
		return err
	}
	return os.WriteFile("/etc/hysteria/config.yaml", config, 0o644)
}

func BuildHysteriaConfig(users []models.User, domain string) ([]byte, error) {
	bundle := make([]hysteriaUser, 0)
	for _, user := range users {
		bundle = append(bundle, hysteriaUser{Name: user.Username, Password: user.Password})
	}

	cfg := hysteriaConfig{
		Listen: ":443",
		TLS: map[string]interface{}{
			"cert": fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", domain),
			"key":  fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", domain),
		},
		Auth: map[string]interface{}{"type": "password"},
		Bandwidth: map[string]interface{}{
			"up":   "200 Mbps",
			"down": "200 Mbps",
		},
		UserBundle: bundle,
	}

	return yaml.Marshal(&cfg)
}

func BuildHysteriaLink(user models.User, domain string) string {
	return fmt.Sprintf("hysteria2://%s@%s:443?insecure=0#%s", user.Password, domain, user.Username)
}
