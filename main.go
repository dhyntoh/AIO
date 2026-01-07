package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"tunnelzero/bot"
	"tunnelzero/installer"
	"tunnelzero/models"
	"tunnelzero/modules"
	"tunnelzero/security"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const validInstallToken = "5407046882"

func main() {
	printLogo()

	reader := bufio.NewReader(os.Stdin)
	if err := promptInstallationToken(reader); err != nil {
		log.Printf("invalid installation token: %v", err)
		os.Exit(1)
	}

	adminID, botToken, domain, err := promptInstallerConfig(reader)
	if err != nil {
		log.Fatalf("failed to read installer config: %v", err)
	}

	db, err := gorm.Open(sqlite.Open("database.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	if err := db.AutoMigrate(&models.User{}, &models.Settings{}); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	settings := models.Settings{AdminID: adminID, BotToken: botToken, Domain: domain}
	if err := db.FirstOrCreate(&settings, models.Settings{Domain: domain}).Error; err != nil {
		log.Fatalf("failed to persist settings: %v", err)
	}

	ctx := context.Background()
	if err := installer.RunSetup(ctx, installer.Config{AdminID: adminID, Domain: domain}); err != nil {
		log.Fatalf("installer failed: %v", err)
	}

	if err := modules.RefreshXrayConfig(db, domain); err != nil {
		log.Printf("xray config refresh failed: %v", err)
	}
	if err := modules.RefreshHysteriaConfig(db, domain); err != nil {
		log.Printf("hysteria config refresh failed: %v", err)
	}

	go startExpirationMonitor(db, settings)

	if err := bot.StartBot(db, bot.Config{AdminID: adminID, Token: botToken, Domain: domain}); err != nil {
		log.Fatalf("bot failed: %v", err)
	}
}

func printLogo() {
	green := "\033[32m"
	reset := "\033[0m"
	fmt.Printf("%s", green)
	fmt.Println("████████╗██╗   ██╗███╗   ██╗███╗   ██╗███████╗██╗     ██╗   ██╗███████╗")
	fmt.Println("╚══██╔══╝██║   ██║████╗  ██║████╗  ██║██╔════╝██║     ██║   ██║██╔════╝")
	fmt.Println("   ██║   ██║   ██║██╔██╗ ██║██╔██╗ ██║█████╗  ██║     ██║   ██║███████╗")
	fmt.Println("   ██║   ██║   ██║██║╚██╗██║██║╚██╗██║██╔══╝  ██║     ██║   ██║╚════██║")
	fmt.Println("   ██║   ╚██████╔╝██║ ╚████║██║ ╚████║███████╗███████╗╚██████╔╝███████║")
	fmt.Println("   ╚═╝    ╚═════╝ ╚═╝  ╚═══╝╚═╝  ╚═══╝╚══════╝╚══════╝ ╚═════╝ ╚══════╝")
	fmt.Printf("%s", reset)
	fmt.Println()
}

func promptInstallationToken(reader *bufio.Reader) error {
	fmt.Print("Enter Installation Token: ")
	token, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token != validInstallToken {
		_ = security.LogAuthFail("Token invalid from IP: 127.0.0.1")
		return errors.New("token mismatch")
	}
	return nil
}

func promptInstallerConfig(reader *bufio.Reader) (int64, string, string, error) {
	fmt.Print("Enter Admin Telegram ID: ")
	adminInput, err := reader.ReadString('\n')
	if err != nil {
		return 0, "", "", err
	}
	adminInput = strings.TrimSpace(adminInput)
	adminID, err := strconv.ParseInt(adminInput, 10, 64)
	if err != nil {
		return 0, "", "", err
	}

	fmt.Print("Enter Telegram Bot Token: ")
	botToken, err := reader.ReadString('\n')
	if err != nil {
		return 0, "", "", err
	}
	botToken = strings.TrimSpace(botToken)

	fmt.Print("Enter Domain/Subdomain: ")
	domain, err := reader.ReadString('\n')
	if err != nil {
		return 0, "", "", err
	}
	domain = strings.TrimSpace(domain)

	if adminID == 0 || botToken == "" || domain == "" {
		return 0, "", "", errors.New("all inputs are required")
	}

	return adminID, botToken, domain, nil
}

func startExpirationMonitor(db *gorm.DB, settings models.Settings) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		var expired []models.User
		if err := db.Where("expired_date < ?", time.Now()).Find(&expired).Error; err != nil {
			log.Printf("expiration check failed: %v", err)
			continue
		}

		if len(expired) == 0 {
			continue
		}

		for _, user := range expired {
			if err := modules.RemoveUser(db, settings.Domain, user); err != nil {
				log.Printf("failed removing expired user %s: %v", user.Username, err)
				continue
			}
			log.Printf("expired user removed: %s", user.Username)
		}
	}
}
