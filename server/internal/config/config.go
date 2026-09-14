// Package config 基于 viper 加载服务配置（config.yaml + 环境变量覆盖）。
package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server   Server   `mapstructure:"server"`
	MySQL    MySQL    `mapstructure:"mysql"`
	Redis    Redis    `mapstructure:"redis"`
	RabbitMQ RabbitMQ `mapstructure:"rabbitmq"`
	LLM      LLM      `mapstructure:"llm"`
	Auth     Auth     `mapstructure:"auth"`
	Task     Task     `mapstructure:"task"`
}

type Server struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"` // debug / release
}

type MySQL struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
}

type Redis struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type RabbitMQ struct {
	URL string `mapstructure:"url"`
}

type LLM struct {
	BaseURL        string  `mapstructure:"base_url"`
	APIKey         string  `mapstructure:"api_key"`
	Model          string  `mapstructure:"model"`
	TimeoutSeconds int     `mapstructure:"timeout_seconds"`
	Temperature    float32 `mapstructure:"temperature"`
	MaxTokens      int     `mapstructure:"max_tokens"`
	RatePerMinute  int     `mapstructure:"rate_per_minute"`
}

type Auth struct {
	JWTSecret     string `mapstructure:"jwt_secret"`
	AdminUser     string `mapstructure:"admin_user"`
	AdminPassword string `mapstructure:"admin_password"`
	TokenHours    int    `mapstructure:"token_hours"`
}

type Task struct {
	WorkDir             string `mapstructure:"workdir"`
	CloneTimeoutSeconds int    `mapstructure:"clone_timeout_seconds"`
	MaxFiles            int    `mapstructure:"max_files"`
	MaxFileKB           int    `mapstructure:"max_file_kb"`
	TotalBudgetKB       int    `mapstructure:"total_budget_kb"`
	ArticleCount        int    `mapstructure:"article_count"`
	Concurrency         int    `mapstructure:"concurrency"`
	MaxRetry            int    `mapstructure:"max_retry"`
}

// Load 读取指定路径的 yaml 配置，环境变量可覆盖同名 key（点号换下划线）。
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件 %s: %w", path, err)
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置: %w", err)
	}
	cfg.applyDefaults()
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.Mode == "" {
		c.Server.Mode = "debug"
	}
	if c.MySQL.Port == 0 {
		c.MySQL.Port = 3306
	}
	if c.Task.WorkDir == "" {
		c.Task.WorkDir = "tmp"
	}
	if c.Task.CloneTimeoutSeconds == 0 {
		c.Task.CloneTimeoutSeconds = 180
	}
	if c.Task.MaxFiles == 0 {
		c.Task.MaxFiles = 30
	}
	if c.Task.MaxFileKB == 0 {
		c.Task.MaxFileKB = 16
	}
	if c.Task.TotalBudgetKB == 0 {
		c.Task.TotalBudgetKB = 120
	}
	if c.Task.ArticleCount == 0 {
		c.Task.ArticleCount = 4
	}
	if c.Task.Concurrency == 0 {
		c.Task.Concurrency = 1
	}
	if c.Task.MaxRetry == 0 {
		c.Task.MaxRetry = 3
	}
	if c.LLM.TimeoutSeconds == 0 {
		c.LLM.TimeoutSeconds = 180
	}
	if c.LLM.MaxTokens == 0 {
		c.LLM.MaxTokens = 8192
	}
	if c.LLM.RatePerMinute == 0 {
		c.LLM.RatePerMinute = 20
	}
	if c.Auth.TokenHours == 0 {
		c.Auth.TokenHours = 72
	}
}
