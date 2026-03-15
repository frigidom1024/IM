package mysql

import (
	"fmt"
	"gochat/pkg/config"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var Client *gorm.DB

func Init(cfg *config.MySQLConfig) error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",

		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})

	if err != nil {
		return fmt.Errorf("open mysql failed: %w", err)
	}

	Client = db
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql db failed: %w", err)
	}

	// 连接池设置
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return nil
}

func Close() error {
	sqlDB, err := Client.DB()
	if err != nil {
		return fmt.Errorf("get sql db failed: %w", err)
	}
	return sqlDB.Close()
}

func GetClient() *gorm.DB {
	return Client
}
