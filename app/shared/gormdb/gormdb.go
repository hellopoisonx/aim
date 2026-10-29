// Package gormdb centralizes GORM connection setup for AIM services.
//
// All services open Postgres through this helper so error translation and
// pool limits stay consistent. Schema changes are applied by the SQL
// migration containers; AutoMigrate is never used.
package gormdb

import (
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Open connects to Postgres with the given DSN. TranslateError is enabled so
// unique/FK violations surface as gorm.ErrDuplicatedKey and friends instead of
// driver-specific error types.
func Open(dsn string) (*gorm.DB, error) {
	logger := gormlogger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), gormlogger.Config{
		SlowThreshold:             time.Second,
		LogLevel:                  gormlogger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  false,
	})

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		TranslateError: true,
		Logger:         logger,
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	return db, nil
}
