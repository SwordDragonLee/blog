// Package service 实现管理端与前台的业务逻辑：登录鉴权、任务管理、
// 文章编辑与发版、门户只读查询（含 Redis 缓存）。
package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"blog/server/internal/config"
	"blog/server/internal/model"

	"github.com/go-sql-driver/mysql"
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

// Profile 查询管理员个人信息（用户名 + 邮箱）。
func (s *AuthService) Profile(ctx context.Context, userID uint) (*model.AdminUser, error) {
	var u model.AdminUser
	err := s.db.WithContext(ctx).First(&u, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: 管理员 %d 不存在", ErrNotFound, userID)
	}
	if err != nil {
		return nil, fmt.Errorf("查询管理员: %w", err)
	}
	return &u, nil
}

// UpdateProfile 更新管理员资料，当前仅支持绑定/修改邮箱（trim + 基本格式校验）。
// 提交未变更的值时 RowsAffected 为 0（MySQL 默认返回变更行数），回读确认存在即视为幂等成功。
func (s *AuthService) UpdateProfile(ctx context.Context, userID uint, email string) (*model.AdminUser, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email != "" && !mailRegex.MatchString(email) {
		return nil, fmt.Errorf("%w: 邮箱格式不正确", ErrInvalid)
	}
	res := s.db.WithContext(ctx).Model(&model.AdminUser{}).
		Where("id = ?", userID).
		Update("email", email)
	if res.Error != nil {
		return nil, fmt.Errorf("更新管理员邮箱: %w", res.Error)
	}
	// RowsAffected 为 0（值未变化或用户不存在）时由 Profile 回读区分，回读 404 才报不存在
	return s.Profile(ctx, userID)
}

// mailRegex 宽松的邮箱格式校验。
var mailRegex = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// UpdateUsername 修改管理员用户名并重签 JWT（JWT Claims 含用户名，改后旧令牌即过期）。
// 用户名占用返回 ErrConflict；查重与更新之间的并发窗口由 uniqueIndex 兜底。
func (s *AuthService) UpdateUsername(ctx context.Context, userID uint, username string) (string, *model.AdminUser, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", nil, fmt.Errorf("%w: 用户名不能为空", ErrInvalid)
	}
	if utf8.RuneCountInString(username) > 64 {
		return "", nil, fmt.Errorf("%w: 用户名长度不能超过 64 个字符", ErrInvalid)
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.AdminUser{}).
		Where("username = ? AND id != ?", username, userID).
		Count(&count).Error; err != nil {
		return "", nil, fmt.Errorf("查询用户名占用: %w", err)
	}
	if count > 0 {
		return "", nil, fmt.Errorf("%w: 用户名已被占用", ErrConflict)
	}
	res := s.db.WithContext(ctx).Model(&model.AdminUser{}).
		Where("id = ?", userID).
		Update("username", username)
	if res.Error != nil {
		// 并发查重窗口内的重名由 uniqueIndex 兜底（MySQL 错误码 1062）
		var mysqlErr *mysql.MySQLError
		if errors.As(res.Error, &mysqlErr) && mysqlErr.Number == 1062 {
			return "", nil, fmt.Errorf("%w: 用户名已被占用", ErrConflict)
		}
		return "", nil, fmt.Errorf("更新用户名: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// MySQL 默认返回变更行数：提交相同用户名时为 0（幂等，继续重签），
		// 用户不存在时也为 0（回读报 404），回读区分两种情况
		user, err := s.Profile(ctx, userID)
		if err != nil {
			return "", nil, err
		}
		if user.Username != username {
			return "", nil, fmt.Errorf("%w: 管理员 %d 不存在", ErrNotFound, userID)
		}
		signed, err := s.sign(*user)
		if err != nil {
			return "", nil, err
		}
		return signed, user, nil
	}
	user, err := s.Profile(ctx, userID)
	if err != nil {
		return "", nil, err
	}
	signed, err := s.sign(*user)
	if err != nil {
		return "", nil, err
	}
	s.log.Info("管理员修改用户名", zap.Uint("user_id", userID), zap.String("username", username))
	return signed, user, nil
}

// ChangePassword 校验旧密码后更新密码哈希。旧密码错误与新密码不合法统一返回
// ErrInvalid（不走 ErrUnauthorized：resp 会映射 401，前端拦截器会全局登出，
// 用户将看不到「旧密码错误」提示）。改密码后旧 JWT 在有效期内仍可用（无状态，
// 不引入 token 版本），前端负责清除登录态强制重登。
func (s *AuthService) ChangePassword(ctx context.Context, userID uint, oldPassword, newPassword string) error {
	newPassword = strings.TrimSpace(newPassword)
	if n := utf8.RuneCountInString(newPassword); n < 6 || n > 64 {
		return fmt.Errorf("%w: 新密码长度需在 6~64 位之间", ErrInvalid)
	}
	var user model.AdminUser
	err := s.db.WithContext(ctx).First(&user, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: 管理员 %d 不存在", ErrNotFound, userID)
	}
	if err != nil {
		return fmt.Errorf("查询管理员: %w", err)
	}
	// 旧密码不做 trim，与 Login 的比对语义一致
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)) != nil {
		return fmt.Errorf("%w: 旧密码错误", ErrInvalid)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("生成密码哈希: %w", err)
	}
	res := s.db.WithContext(ctx).Model(&model.AdminUser{}).
		Where("id = ?", userID).
		Update("password_hash", string(hash))
	if res.Error != nil {
		return fmt.Errorf("更新密码: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("%w: 管理员 %d 不存在", ErrNotFound, userID)
	}
	s.log.Info("管理员修改密码", zap.Uint("user_id", userID))
	return nil
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
