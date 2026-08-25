package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/gogf/gf/v2/frame/g"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var (
	ErrMysqlDSNRequired = errors.New("mysql data source is not configured")
	ErrMysqlWriteDenied = errors.New("mysql tool is read-only")
	ErrMysqlSQLInvalid  = errors.New("mysql SQL must be one read-only statement")
)

type MysqlCrudInput struct {
	SQL string `json:"sql" jsonschema:"description=Read-only SQL query. Only SELECT, SHOW, DESCRIBE, or EXPLAIN statements are allowed"`
}

type MysqlCrudConfig struct {
	DSN     string
	Timeout time.Duration
}

func LoadMysqlCrudConfig(ctx context.Context) MysqlCrudConfig {
	cfg := MysqlCrudConfig{Timeout: 5 * time.Second}
	if value, err := g.Cfg().Get(ctx, "mysql_data_source.dsn"); err == nil && !value.IsNil() {
		cfg.DSN = strings.TrimSpace(value.String())
	}
	if value, err := g.Cfg().Get(ctx, "mysql_data_source.timeout"); err == nil && !value.IsNil() {
		if timeout, parseErr := time.ParseDuration(value.String()); parseErr == nil && timeout > 0 {
			cfg.Timeout = timeout
		}
	}
	return cfg
}

func NewMysqlCrudTool() (tool.InvokableTool, error) {
	return NewMysqlCrudToolWithConfig(LoadMysqlCrudConfig(context.Background()))
}

func NewMysqlCrudToolWithConfig(cfg MysqlCrudConfig) (tool.InvokableTool, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	var db *gorm.DB
	if strings.TrimSpace(cfg.DSN) != "" {
		var err error
		db, err = gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{})
		if err != nil {
			return nil, fmt.Errorf("open mysql data source: %w", err)
		}
	}
	return utils.InferOptionableTool(
		"mysql_crud",
		"Query a server-configured MySQL data source. The tool is read-only and accepts one SELECT, SHOW, DESCRIBE, or EXPLAIN statement.",
		func(ctx context.Context, input *MysqlCrudInput, opts ...tool.Option) (string, error) {
			if input == nil {
				return "", errors.New("mysql input is required")
			}
			if err := validateReadOnlySQL(input.SQL); err != nil {
				return "", err
			}
			if db == nil {
				return "", ErrMysqlDSNRequired
			}
			queryCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
			defer cancel()
			var results []map[string]any
			if err := db.WithContext(queryCtx).Raw(input.SQL).Scan(&results).Error; err != nil {
				return "", err
			}
			payload, err := json.Marshal(results)
			return string(payload), err
		})
}

func validateReadOnlySQL(query string) error {
	query = strings.TrimSpace(query)
	if query == "" || strings.Contains(query, ";") {
		return ErrMysqlSQLInvalid
	}
	fields := strings.Fields(strings.ToUpper(query))
	if len(fields) == 0 {
		return ErrMysqlSQLInvalid
	}
	switch fields[0] {
	case "SELECT", "SHOW", "DESCRIBE", "DESC", "EXPLAIN":
		return nil
	default:
		return ErrMysqlWriteDenied
	}
}
