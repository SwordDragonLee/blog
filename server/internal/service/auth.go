// Package service 实现管理端与前台的业务逻辑：登录鉴权、任务管理、
// 文章编辑与发版、门户只读查询（含 Redis 缓存）。
package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"blog/server/internal/config"
	"blog/server/internal/model"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// 业务错误哨兵：HTTP 层据此映射状态码。
var (
	ErrInvalid      = errors.New("参数不合法")
	ErrUnauthorized = errors.New("未登录或登录已过期")
	ErrNotFound     = errors.New("资源不存在")
	ErrConflict     = errors.New("状态冲突")
)

// AuthService 登录校验与 JWT 签发/解析。
type AuthService struct {
	db  *gorm.DB
	cfg config.Auth
	log *zap.Logger
}

// NewAuthService 创建认证服务。
func NewAuthService(db *gorm.DB, cfg config.Auth, log *zap.Logger) *AuthService {
	if cfg.TokenHours <= 0 {
		cfg.TokenHours = 72
	}
	return &AuthService{db: db, cfg: cfg, log: log}
}

// Claims JWT 载荷：管理员 id + 用户名 + 标准注册声明。
type Claims struct {
	UserID   uint   `json:"uid"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Login 校验用户名密码（bcrypt），签发 HS256 JWT。
// 密码错误统一返回 ErrUnauthorized，避免泄露账号是否存在。
func (s *AuthService) Login(ctx context.Context, username, password string) (string, *model.AdminUser, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return "", nil, fmt.Errorf("%w: 用户名和密码不能为空", ErrInvalid)
	}
	var user model.AdminUser
	err := s.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil, fmt.Errorf("%w: 用户名或密码错误", ErrUnauthorized)
	}
	if err != nil {
		return "", nil, fmt.Errorf("查询管理员: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", nil, fmt.Errorf("%w: 用户名或密码错误", ErrUnauthorized)
	}
	signed, err := s.sign(user)
	if err != nil {
		return "", nil, err
	}
	s.log.Info("管理员登录成功", zap.Uint("user_id", user.ID), zap.String("username", user.Username))
	return signed, &user, nil
}

// ParseToken 校验并解析 JWT（仅接受 HS256）。
func (s *AuthService) ParseToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		return []byte(s.cfg.JWTSecret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnauthorized, err)
	}
	return claims, nil
}

// sign 为管理员签发 HS256 JWT，时效取 cfg.Auth.TokenHours。
func (s *AuthService) sign(user model.AdminUser) (string, error) {
	if strings.TrimSpace(s.cfg.JWTSecret) == "" {
		return "", fmt.Errorf("auth.jwt_secret 未配置，无法签发令牌")
	}
	now := time.Now()
	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "blog-server",
			Subject:   strconv.FormatUint(uint64(user.ID), 10),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(s.cfg.TokenHours) * time.Hour)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("签发 JWT: %w", err)
	}
	return signed, nil
}
