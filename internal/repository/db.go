// Package repository 数据访问层。[L2]
//
// 【硬约束】只有这个包能持有 *gorm.DB。service 和 handler 一律通过
// repository 暴露的方法访问数据库，不要把 *gorm.DB 传出去。
package repository

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ruoyi-go/internal/config"
	"ruoyi-go/pkg/types"
)

var db *gorm.DB

// Init 初始化数据库连接池。
func Init(cfg config.MySQLConfig) error {
	gormLogger := logger.New(slogWriter{}, logger.Config{
		SlowThreshold:             cfg.SlowThreshold,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  false,
	})

	dsn, err := drivermysql.ParseDSN(cfg.DSN)
	if err != nil {
		return fmt.Errorf("解析 MySQL DSN 失败: %w", err)
	}
	dsn.Timeout = cfg.ConnectTimeout
	dsn.ReadTimeout = cfg.ReadTimeout
	dsn.WriteTimeout = cfg.WriteTimeout
	// types.Time.Scan requires native time.Time values. Force both options so a
	// future DSN edit cannot pass Ping and fail only on the first business query.
	dsn.ParseTime = true
	dsn.Loc = types.Location
	// RowsAffected 按匹配行而不是实际变化行统计，幂等更新仍能区分“存在”与“不存在”。
	dsn.ClientFoundRows = true

	gdb, err := gorm.Open(mysql.Open(dsn.FormatDSN()), &gorm.Config{
		Logger: gormLogger,
		// 单条写操作不自动开事务，事务边界由 service 层显式控制
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return fmt.Errorf("连接 MySQL 失败: %w", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return fmt.Errorf("获取底层连接池失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		return fmt.Errorf("MySQL ping 失败: %w", err)
	}

	db = gdb
	return nil
}

// DB 返回绑定了 ctx 的会话。
//
// 所有查询都必须走这里，不要用裸的 db —— ctx 携带超时和取消信号。
func DB(ctx context.Context) *gorm.DB {
	return db.WithContext(ctx)
}

// Ping 探活，供健康检查用。
//
// 走 PingContext 而不是 SELECT 1：前者能感知连接池本身的状态，
// 且不占用查询计划缓存。
func Ping(ctx context.Context) error {
	if db == nil {
		return fmt.Errorf("数据库未初始化")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Stats 返回连接池状态，供健康检查和排查连接泄漏用。
//
// InUse 长期贴着 MaxOpen、WaitCount 持续增长，就是连接没归还或并发超了。
func Stats(ctx context.Context) (map[string]any, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	s := sqlDB.Stats()
	return map[string]any{
		"maxOpen":      s.MaxOpenConnections,
		"open":         s.OpenConnections,
		"inUse":        s.InUse,
		"idle":         s.Idle,
		"waitCount":    s.WaitCount,
		"waitMs":       s.WaitDuration.Milliseconds(),
		"maxIdleClose": s.MaxIdleClosed,
	}, nil
}

// Transaction 在事务中执行 fn。
func Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return db.WithContext(ctx).Transaction(fn)
}

// Close 关闭连接池。
func Close() error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// slogWriter 把 GORM 日志接到标准库 slog。
type slogWriter struct{}

func (slogWriter) Printf(format string, args ...any) {
	slog.Warn(fmt.Sprintf(format, args...))
}
