package mysql

import (
	"fmt"
	"time"

	"gochat/internal/pkg/config"

	drivermysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var Client *gorm.DB

func Init(cfg *config.MySQLConfig) error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	db, err := gorm.Open(drivermysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("open mysql failed: %w", err)
	}

	Client = db
	sqlDB, err := db.DB()
	if err != nil {
		Client = nil
		return fmt.Errorf("get sql db failed: %w", err)
	}

	// 连接池设置
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		Client = nil
		return fmt.Errorf("ping mysql failed: %w", err)
	}
	return nil
}

func Close() error {
	if Client == nil {
		return nil
	}
	sqlDB, err := Client.DB()
	if err != nil {
		return fmt.Errorf("get sql db failed: %w", err)
	}
	Client = nil
	return sqlDB.Close()
}

func GetClient() *gorm.DB {
	return Client
}
