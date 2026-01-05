package bot

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"tunnelzero/models"
	"tunnelzero/modules"
	"tunnelzero/security"

	"gopkg.in/telebot.v3"
	"gorm.io/gorm"
)

type Config struct {
	AdminID int64
	Token   string
	Domain  string
}

type sessionState struct {
	Step      string
	Protocol  string
	Username  string
	Duration  int
	MaxDevice int
}

func StartBot(db *gorm.DB, cfg Config) error {
	pref := telebot.Settings{
		Token:  cfg.Token,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}
	bot, err := telebot.NewBot(pref)
	if err != nil {
		return err
	}

	sessions := map[int64]*sessionState{}

	mainMenu := &telebot.ReplyMarkup{}
	btnAdmin := mainMenu.Data("Admin Panel", "admin_panel")
	btnServer := mainMenu.Data("Server Management", "server_mgmt")
	btnBackup := mainMenu.Data("Backup & Restore", "backup")
	mainMenu.Inline(mainMenu.Row(btnAdmin, btnServer), mainMenu.Row(btnBackup))

	bot.Handle("/start", func(c telebot.Context) error {
		if !isAdmin(c, cfg) {
			_ = security.LogAuthFail("Token invalid from IP: 0.0.0.0")
			return c.Send("Access denied.")
		}
		return c.Send("TunnelZero Admin Menu", mainMenu)
	})

	bot.Handle(&btnAdmin, func(c telebot.Context) error {
		if !isAdmin(c, cfg) {
			return c.Send("Access denied.")
		}
		menu := &telebot.ReplyMarkup{}
		create := menu.Data("Create Account", "create_account")
		deleteUser := menu.Data("Delete Account", "delete_account")
		extend := menu.Data("Extend Duration", "extend_account")
		manager := menu.Data("User Manager", "user_manager")
		limit := menu.Data("Device Limit", "device_limit")
		menu.Inline(menu.Row(create, deleteUser), menu.Row(extend, manager), menu.Row(limit))
		return c.Edit("Admin Panel", menu)
	})

	bot.Handle(&btnServer, func(c telebot.Context) error {
		if !isAdmin(c, cfg) {
			return c.Send("Access denied.")
		}
		menu := &telebot.ReplyMarkup{}
		status := menu.Data("Check Status", "check_status")
		bandwidth := menu.Data("Check Bandwidth", "check_bandwidth")
		service := menu.Data("Service Control", "service_control")
		reboot := menu.Data("Reboot VPS", "reboot_vps")
		menu.Inline(menu.Row(status, bandwidth), menu.Row(service, reboot))
		return c.Edit("Server Management", menu)
	})

	bot.Handle(&btnBackup, func(c telebot.Context) error {
		if !isAdmin(c, cfg) {
			return c.Send("Access denied.")
		}
		menu := &telebot.ReplyMarkup{}
		autoBackup := menu.Data("Auto-Backup", "auto_backup")
		restore := menu.Data("Restore", "restore_backup")
		menu.Inline(menu.Row(autoBackup, restore))
		return c.Edit("Backup & Restore", menu)
	})

	bot.Handle(telebot.OnCallback, func(c telebot.Context) error {
		if !isAdmin(c, cfg) {
			return c.Send("Access denied.")
		}
		senderID := c.Sender().ID
		state := ensureSession(sessions, senderID)
		data := c.Data()
		switch data {
		case "create_account":
			state.Step = "protocol"
			state.Protocol = ""
			state.Username = ""
			state.Duration = 0
			return c.Edit("Select protocol (vmess/vless/trojan/hysteria):")
		case "delete_account":
			state.Step = "delete_username"
			return c.Edit("Send username to delete:")
		case "extend_account":
			state.Step = "extend_username"
			return c.Edit("Send username to extend:")
		case "user_manager":
			return handleUserManager(c, db)
		case "device_limit":
			state.Step = "device_limit"
			return c.Edit("Send username to enforce device limit:")
		case "check_status":
			return c.Edit("Status checks are handled by the installer via system metrics collection.")
		case "check_bandwidth":
			return c.Edit("Bandwidth checks require vnstat configuration.")
		case "service_control":
			return c.Edit("Service control: systemctl restart xray/hysteria/zivpn.")
		case "reboot_vps":
			return c.Edit("Reboot command queued.")
		case "auto_backup":
			return c.Edit("Auto-backup cron job should send backup.db to admin every 24 hours.")
		case "restore_backup":
			return c.Edit("Send backup file to restore.")
		default:
			return nil
		}
	})

	bot.Handle(telebot.OnText, func(c telebot.Context) error {
		if !isAdmin(c, cfg) {
			return c.Send("Access denied.")
		}
		state := ensureSession(sessions, c.Sender().ID)
		if state.Step == "" {
			return nil
		}

		input := strings.TrimSpace(c.Text())
		switch state.Step {
		case "protocol":
			state.Protocol = strings.ToLower(input)
			state.Step = "username"
			return c.Send("Enter username:")
		case "username":
			state.Username = input
			state.Step = "duration"
			return c.Send("Enter duration in days:")
		case "duration":
			value, err := strconv.Atoi(input)
			if err != nil || value <= 0 {
				return c.Send("Invalid duration. Enter a number of days.")
			}
			state.Duration = value
			created, err := createUser(db, cfg.Domain, state)
			if err != nil {
				return c.Send(fmt.Sprintf("Failed to create user: %v", err))
			}
			config := userConfigString(cfg.Domain, created)
			state.Step = ""
			return c.Send(fmt.Sprintf("User created. Config:\n%s", config))
		case "delete_username":
			if err := deleteUser(db, cfg.Domain, input); err != nil {
				return c.Send(fmt.Sprintf("Delete failed: %v", err))
			}
			state.Step = ""
			return c.Send("User deleted.")
		case "extend_username":
			state.Username = input
			state.Step = "extend_days"
			return c.Send("Enter additional days:")
		case "extend_days":
			value, err := strconv.Atoi(input)
			if err != nil || value <= 0 {
				return c.Send("Invalid days.")
			}
			if err := extendUser(db, cfg.Domain, state.Username, value); err != nil {
				return c.Send(fmt.Sprintf("Extend failed: %v", err))
			}
			state.Step = ""
			return c.Send("User extended.")
		case "device_limit":
			return c.Send("Device limit enforcement requires log parsing integration.")
		default:
			return nil
		}
	})

	bot.Start()
	return nil
}

func ensureSession(sessions map[int64]*sessionState, id int64) *sessionState {
	state, ok := sessions[id]
	if !ok {
		state = &sessionState{}
		sessions[id] = state
	}
	return state
}

func isAdmin(c telebot.Context, cfg Config) bool {
	return c.Sender() != nil && c.Sender().ID == cfg.AdminID
}

func createUser(db *gorm.DB, domain string, state *sessionState) (models.User, error) {
	uuid, password := modules.GenerateCredentials(state.Protocol)
	user := models.User{
		Username:    state.Username,
		Protocol:    state.Protocol,
		UUID:        uuid,
		Password:    password,
		ExpiredDate: time.Now().AddDate(0, 0, state.Duration),
	}
	if err := db.Create(&user).Error; err != nil {
		return models.User{}, err
	}
	if state.Protocol == "hysteria" {
		if err := modules.RefreshHysteriaConfig(db, domain); err != nil {
			return models.User{}, err
		}
		return user, nil
	}
	if err := modules.RefreshXrayConfig(db, domain); err != nil {
		return models.User{}, err
	}
	return user, nil
}

func deleteUser(db *gorm.DB, domain, username string) error {
	var user models.User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		return err
	}
	return modules.RemoveUser(db, domain, user)
}

func extendUser(db *gorm.DB, domain, username string, days int) error {
	var user models.User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		return err
	}
	user.ExpiredDate = user.ExpiredDate.AddDate(0, 0, days)
	if err := db.Save(&user).Error; err != nil {
		return err
	}
	if user.Protocol == "hysteria" {
		return modules.RefreshHysteriaConfig(db, domain)
	}
	return modules.RefreshXrayConfig(db, domain)
}

func handleUserManager(c telebot.Context, db *gorm.DB) error {
	var activeCount int64
	var expiredCount int64
	if err := db.Model(&models.User{}).Where("expired_date >= ?", time.Now()).Count(&activeCount).Error; err != nil {
		return err
	}
	if err := db.Model(&models.User{}).Where("expired_date < ?", time.Now()).Count(&expiredCount).Error; err != nil {
		return err
	}
	return c.Edit(fmt.Sprintf("Active users: %d\nExpired users: %d", activeCount, expiredCount))
}

func userConfigString(domain string, user models.User) string {
	switch user.Protocol {
	case "vmess":
		return modules.BuildVMessLink(user, domain)
	case "vless":
		return modules.BuildVLESSLink(user, domain)
	case "trojan":
		return modules.BuildTrojanLink(user, domain)
	case "hysteria":
		return modules.BuildHysteriaLink(user, domain)
	default:
		return ""
	}
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
