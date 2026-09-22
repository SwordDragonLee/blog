// Package config 基于 viper 加载服务配置（config.yaml + 环境变量覆盖）。
package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
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
	Email    Email    `mapstructure:"email"`
	Qdrant   Qdrant   `mapstructure:"qdrant"`
	RAG      RAG      `mapstructure:"rag"`
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
	Proxy               string `mapstructure:"proxy"` // 克隆走 HTTP 代理（如 http://127.0.0.1:7890），留空直连
	MaxFiles            int    `mapstructure:"max_files"`
	MaxFileKB           int    `mapstructure:"max_file_kb"`
	TotalBudgetKB       int    `mapstructure:"total_budget_kb"`
	ArticleCount        int    `mapstructure:"article_count"`
	Concurrency         int    `mapstructure:"concurrency"`
	MaxRetry            int    `mapstructure:"max_retry"`
}

// Email SMTP 邮件通知配置（发布成功后通知发布者）。
type Email struct {
	Enabled  bool   `mapstructure:"enabled"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"` // 默认 465（SSL）
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"` // SMTP 授权码，非邮箱登录密码
	From     string `mapstructure:"from"`     // 发件人，缺省等于 username
	SiteURL  string `mapstructure:"site_url"` // 前台博客地址，用于拼文章链接
}

// Qdrant 向量数据库连接配置（RAG 技术问答）。
type Qdrant struct {
	BaseURL    string `mapstructure:"base_url"`   // REST 地址，如 http://localhost:6333
	Collection string `mapstructure:"collection"` // 集合名，缺省 blog_articles
}

// RAG 技术问答配置：切块、检索与 embedding 参数；
// chat_* 为问答专用对话模型（面向公众、调用量大，建议免费模型），不配置则复用主模型。
type RAG struct {
	Enabled             bool   `mapstructure:"enabled"`
	EmbeddingBaseURL    string `mapstructure:"embedding_base_url"`   // 空 = 复用 llm.base_url
	EmbeddingAPIKey     string `mapstructure:"embedding_api_key"`    // 空 = 复用 llm.api_key
	EmbeddingModel      string `mapstructure:"embedding_model"`      // 缺省 embedding-3
	EmbeddingDimensions int    `mapstructure:"embedding_dimensions"` // 向量维度，缺省 2048
	// EmbeddingSendDimensions 是否随请求发送 dimensions 参数：智谱 embedding-3 支持指定维度需传 true；
	// BAAI/bge-m3 等固定维度模型不认该参数（传了报 400），保持 false
	EmbeddingSendDimensions bool    `mapstructure:"embedding_send_dimensions"`
	ChatModel               string  `mapstructure:"chat_model"`      // 空 = 复用 llm 主模型
	ChatBaseURL             string  `mapstructure:"chat_base_url"`   // 空 = 复用 llm.base_url
	ChatAPIKey              string  `mapstructure:"chat_api_key"`    // 空 = 复用 llm.api_key
	ChatMaxTokens           int     `mapstructure:"chat_max_tokens"` // 单次回答长度上限，缺省 2048
	ChunkSize               int     `mapstructure:"chunk_size"`      // 切块目标字符数，缺省 800
	ChunkOverlap            int     `mapstructure:"chunk_overlap"`   // 相邻块重叠字符数，缺省 100
	TopK                    int     `mapstructure:"top_k"`           // 检索条数，缺省 5
	ScoreThreshold          float32 `mapstructure:"score_threshold"` // 相似度阈值，低于丢弃，缺省 0.3
}

// Load 读取指定路径的 yaml 配置。config.yaml 只放非私密配置；密钥/密码等私密项经环境变量注入。
//
// .env 并非 viper 直接读取，而是两段接力：
//  1. godotenv 把同目录 .env 的 KEY=VALUE 逐行塞进进程环境变量（文件不存在则静默跳过，
//     如生产容器里没有该文件，由 compose 的 env_file 直接注入进程环境，效果等价）
//  2. viper 查每个配置键时按「点号换下划线 + 全大写」变换出环境变量名去进程环境里找，
//     命中则覆盖 yaml 值（如 llm.api_key → 查 LLM_API_KEY）
//
// 优先级：进程环境变量（含 compose 注入）> .env > config.yaml。
// 注意：viper 的 Unmarshal 只遍历 yaml 中已声明的键——yaml 里的空占位（如 api_key: ""）
// 是承重墙，删掉后对应的环境变量将无法映射进结构体，且全程不报错。
func Load(path string) (*Config, error) {
	// 先于 viper 读配置执行：.env 灌进进程环境，供下方 AutomaticEnv 命中；
	// godotenv.Load 默认不覆盖已存在的环境变量，故外部注入优先于 .env 文件
	_ = godotenv.Load(filepath.Join(filepath.Dir(path), ".env"))

	v := viper.New()
	v.SetConfigFile(path)
	// 环境变量覆盖 yaml：「. → _」由 replacer 负责，大写由 AutomaticEnv 负责；
	// 环境优先级高于配置文件，这是密钥不落 yaml 的实现基础
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
	if c.Email.Port == 0 {
		c.Email.Port = 465
	}
	if c.Email.From == "" {
		c.Email.From = c.Email.Username
	}
	if c.Qdrant.Collection == "" {
		c.Qdrant.Collection = "blog_articles"
	}
	if c.RAG.EmbeddingModel == "" {
		c.RAG.EmbeddingModel = "embedding-3"
	}
	if c.RAG.EmbeddingDimensions == 0 {
		c.RAG.EmbeddingDimensions = 2048
	}
	if c.RAG.ChunkSize == 0 {
		c.RAG.ChunkSize = 800
	}
	if c.RAG.ChunkOverlap == 0 {
		c.RAG.ChunkOverlap = 100
	}
	if c.RAG.TopK == 0 {
		c.RAG.TopK = 5
	}
	if c.RAG.ChatMaxTokens == 0 {
		c.RAG.ChatMaxTokens = 2048
	}
	if c.RAG.ScoreThreshold == 0 {
		c.RAG.ScoreThreshold = 0.3
	}
}
