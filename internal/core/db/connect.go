package db

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Connect 建立数据库连接并自动迁移
func Connect(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("db migrate: %w", err)
	}
	return db, nil
}

// migrate 自动创建/更新所有表
func migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		// 三层消息结构
		&GameSession{},
		&Floor{},
		&MessagePage{},
		// 记忆系统
		&Memory{},
		// 创作层
		&CharacterCard{},
		&GameTemplate{},
		&WorldbookEntry{},
		&PresetEntry{},
		// LLM 配置层
		&LLMProfile{},
		&LLMProfileBinding{},
		// Regex 后处理系统
		&RegexProfile{},
		&RegexRule{},
		// 素材库
		&Material{},
		// 工具执行记录
		&ToolExecutionRecord{},
		// 用户自定义工具
		&PresetTool{},
	)
}
