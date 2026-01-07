package models

import "time"

type User struct {
	ID          uint `gorm:"primaryKey"`
	Username    string
	Protocol    string
	UUID        string
	Password    string
	ExpiredDate time.Time `gorm:"column:expired_date"`
	MaxDevices  int
	IsBanned    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Settings struct {
	ID        uint `gorm:"primaryKey"`
	AdminID   int64
	BotToken  string
	Domain    string
	CreatedAt time.Time
	UpdatedAt time.Time
}
