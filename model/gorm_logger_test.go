package model

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestSanitizeDBErrorNeverReturnsDriverOrWrappedValues(t *testing.T) {
	const secret = "synthetic-bound-secret"
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "mysql",
			err:  &mysql.MySQLError{Number: 1062, Message: "Duplicate entry '" + secret + "'"},
			want: "mysql error 1062",
		},
		{
			name: "postgres",
			err:  &pgconn.PgError{Code: "23505", Message: "duplicate key", Detail: "Key (k)=(" + secret + ") exists"},
			want: "postgres error SQLSTATE 23505",
		},
		{
			name: "wrapped gorm sentinel",
			err:  fmt.Errorf("%w: value=%s", gorm.ErrInvalidValue, secret),
			want: "database operation failed",
		},
		{
			name: "unknown database error",
			err:  fmt.Errorf("constraint rejected value %s", secret),
			want: "database operation failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := sanitizeDBError(test.err)
			require.Error(t, got)
			assert.Equal(t, test.want, got.Error())
			assert.NotContains(t, got.Error(), secret)
		})
	}
	assert.ErrorIs(t, sanitizeDBError(fmt.Errorf("wrapped: %w", context.Canceled)), context.Canceled)
	assert.ErrorIs(t, sanitizeDBError(fmt.Errorf("wrapped: %w", context.DeadlineExceeded)), context.DeadlineExceeded)
}

func TestSanitizedGormWriterRemovesSyntheticSentinelValue(t *testing.T) {
	var output bytes.Buffer
	writer := &sanitizedGormLogWriter{delegate: newTestStdLogger(&output)}
	writer.Printf("%v", fmt.Errorf("%w: synthetic-bound-secret", gorm.ErrInvalidData))

	assert.Contains(t, output.String(), "database operation failed")
	assert.NotContains(t, output.String(), "synthetic-bound-secret")
}

func newTestStdLogger(output *bytes.Buffer) *log.Logger {
	return log.New(output, "", 0)
}

func TestGormLoggerRedactsNormalSlowErrorAndDebugSQL(t *testing.T) {
	const secret = "synthetic-bound-secret"

	tests := []struct {
		name          string
		threshold     time.Duration
		configure     func(*gorm.DB) *gorm.DB
		query         string
		wantLogMarker string
	}{
		{
			name:          "normal info",
			threshold:     time.Hour,
			configure:     func(db *gorm.DB) *gorm.DB { return db.Session(&gorm.Session{Logger: db.Logger.LogMode(gormlogger.Info)}) },
			query:         "SELECT ? AS value",
			wantLogMarker: "SELECT ? AS value",
		},
		{
			name:          "slow",
			threshold:     time.Nanosecond,
			configure:     func(db *gorm.DB) *gorm.DB { return db },
			query:         "SELECT ? AS value",
			wantLogMarker: "SLOW SQL",
		},
		{
			name:          "error",
			threshold:     time.Hour,
			configure:     func(db *gorm.DB) *gorm.DB { return db },
			query:         "SELECT * FROM missing_table WHERE value = ?",
			wantLogMarker: "sqlite error",
		},
		{
			name:          "debug",
			threshold:     time.Hour,
			configure:     func(db *gorm.DB) *gorm.DB { return db.Debug() },
			query:         "SELECT ? AS value",
			wantLogMarker: "SELECT ? AS value",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
				Logger: newGormLoggerWithThreshold(&output, test.threshold),
			})
			require.NoError(t, err)
			output.Reset()

			var value string
			query := test.configure(database).Raw(test.query, secret).Scan(&value)
			if strings.Contains(test.query, "missing_table") {
				require.Error(t, query.Error)
			} else {
				require.NoError(t, query.Error)
			}

			logged := output.String()
			assert.Contains(t, logged, test.wantLogMarker)
			assert.NotContains(t, logged, secret)
			assert.Contains(t, logged, "?")
		})
	}
}
