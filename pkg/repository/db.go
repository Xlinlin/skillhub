package repository

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/cinience/skillhub/pkg/config"
	"github.com/cinience/skillhub/pkg/model"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func NewDB(cfg config.DatabaseConfig) (*gorm.DB, error) {
	gormCfg := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	}

	var db *gorm.DB
	var err error

	switch cfg.Driver {
	case "postgres":
		db, err = gorm.Open(postgres.Open(cfg.URL), gormCfg)
	case "sqlite", "":
		dbPath := cfg.URL
		if dbPath == "" {
			dbPath = "./data/skillhub.db"
		}
		if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
		path, rawQuery, _ := strings.Cut(dbPath, "?")
		query, parseErr := url.ParseQuery(rawQuery)
		if parseErr != nil {
			return nil, fmt.Errorf("parse sqlite options: %w", parseErr)
		}
		query.Add("_pragma", "journal_mode(WAL)")
		query.Add("_pragma", "busy_timeout(5000)")
		dsn := path + "?" + query.Encode()
		db, err = gorm.Open(sqlite.Open(dsn), gormCfg)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get underlying db: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)

	// AutoMigrate all models
	if err := db.AutoMigrate(
		&model.User{},
		&model.Skill{},
		&model.AgentPackage{},
		&model.AgentPackageVersion{},
		&model.SkillVersion{},
		&model.SkillSlugAlias{},
		&model.APIToken{},
		&model.Star{},
		&model.DownloadDedup{},
		&model.AuditLog{},
		&model.SkillDailyStats{},
		&model.ReservedSlug{},
		&model.SkillOwnershipTransfer{},
		&model.Comment{},
	); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}

	// Seed reserved slugs
	seedReservedSlugs(db)

	return db, nil
}

func seedReservedSlugs(db *gorm.DB) {
	slugs := []string{"admin", "api", "auth", "login", "register", "search", "settings", "system", "help", "about"}
	reason := "system reserved"
	for _, slug := range slugs {
		db.Where("slug = ?", slug).FirstOrCreate(&model.ReservedSlug{
			Slug:   slug,
			Reason: &reason,
		})
	}
	log.Println("database: reserved slugs seeded")
}
