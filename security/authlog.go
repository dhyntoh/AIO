package security

import (
	"fmt"
	"os"
	"time"
)

const authLogPath = "/var/log/tunnelzero-auth.log"

func LogAuthFail(message string) error {
	timestamp := time.Now().Format(time.RFC3339)
	entry := fmt.Sprintf("[%s] [AUTH_FAIL] %s\n", timestamp, message)
	file, err := os.OpenFile(authLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(entry)
	return err
}
