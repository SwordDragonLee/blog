package model

import "time"

// AdminUser 管理员账号（单管理员，启动时种子写入）。
type AdminUser struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"size:64;uniqueIndex" json:"username"`
	PasswordHash string    `gorm:"size:128" json:"-"`
	Email        string    `gorm:"size:128;default:''" json:"email"`
	CreatedAt    time.Time `json:"created_at"`
}

func (AdminUser) TableName() string { return "admin_user" }
