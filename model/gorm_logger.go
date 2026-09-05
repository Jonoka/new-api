package model

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/QuantumNous/new-api/common"
	sqlitedriver "github.com/glebarez/go-sqlite"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	defaultSlowThresholdMs = 200
	maxSlowThresholdMs     = 60 * 60 * 1000
)

func newGormConfig(prepareStmt bool) *gorm.Config {
	return &gorm.Config{
		PrepareStmt: prepareStmt,
		Logger:      newGormLogger(os.Stdout),
		Plugins: map[string]gorm.Plugin{
			"newapi:parameterized-row-logging": parameterizedRowLogging{},
		},
	}
}

type parameterizedRowLogging struct{}

func (parameterizedRowLogging) Name() string { return "newapi:parameterized-row-logging" }

func (plugin parameterizedRowLogging) Initialize(db *gorm.DB) error {
	return db.Callback().Row().Before("gorm:row").Register(plugin.Name(), func(tx *gorm.DB) {
		// GORM 1.25.2 Scan replaces the logger with a recorder that loses
		// ParamsFilter. Preserve placeholders before that recorder sees SQL.
		if _, ok := tx.Logger.(gorm.ParamsFilter); !ok {
			config := *tx.Config
			config.Logger = parameterizedScanLogger{Interface: tx.Logger}
			tx.Config = &config
		}
	})
}

type parameterizedScanLogger struct {
	gormlogger.Interface
}

func (parameterizedScanLogger) ParamsFilter(_ context.Context, sql string, _ ...interface{}) (string, []interface{}) {
	return sql, nil
}

func newGormLogger(w io.Writer) gormlogger.Interface {
	slowThresholdMs := common.GetEnvOrDefault("SQL_SLOW_THRESHOLD_MS", defaultSlowThresholdMs)
	if slowThresholdMs < 0 || slowThresholdMs > maxSlowThresholdMs {
		common.SysError(fmt.Sprintf("invalid SQL_SLOW_THRESHOLD_MS %d (allowed 0-%d, 0 disables slow query logging), using default %d", slowThresholdMs, maxSlowThresholdMs, defaultSlowThresholdMs))
		slowThresholdMs = defaultSlowThresholdMs
	}
	return newGormLoggerWithThreshold(w, time.Duration(slowThresholdMs)*time.Millisecond)
}

func newGormLoggerWithThreshold(w io.Writer, slowThreshold time.Duration) gormlogger.Interface {
	return gormlogger.New(&sanitizedGormLogWriter{delegate: log.New(w, "\r\n", log.LstdFlags)}, gormlogger.Config{
		SlowThreshold:             slowThreshold,
		LogLevel:                  gormlogger.Warn,
		IgnoreRecordNotFoundError: true,
		ParameterizedQueries:      true,
		Colorful:                  true,
	})
}

// GORM parameterization protects the SQL string. Driver and wrapped errors can
// independently contain bound values, so the writer also replaces error text
// with a stable category before it reaches any log level.
type sanitizedGormLogWriter struct {
	delegate *log.Logger
}

func (s *sanitizedGormLogWriter) Printf(format string, args ...interface{}) {
	for i, arg := range args {
		if err, ok := arg.(error); ok {
			args[i] = sanitizeDBError(err)
		}
	}
	s.delegate.Printf(format, args...)
}

func sanitizeDBError(err error) error {
	if err == nil {
		return nil
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return fmt.Errorf("mysql error %d", mysqlErr.Number)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return fmt.Errorf("postgres error SQLSTATE %s", pgErr.Code)
	}
	var sqliteErr *sqlitedriver.Error
	if errors.As(err, &sqliteErr) {
		return fmt.Errorf("sqlite error %d", sqliteErr.Code())
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return gorm.ErrRecordNotFound
	}
	return errors.New("database operation failed")
}
