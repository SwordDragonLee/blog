// Package mailer 邮件通知：发布成功后给发布者绑定的邮箱发通知邮件。
// 发送在独立 goroutine 中进行，任何失败只记日志、绝不影响发布主流程。
package mailer

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"blog/server/internal/config"
	"blog/server/internal/model"

	"github.com/wneessen/go-mail"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// sendTimeout 单封邮件发送超时。
const sendTimeout = 10 * time.Second

// Mailer SMTP 邮件发送器。
type Mailer struct {
	db  *gorm.DB
	cfg config.Email
	log *zap.Logger
}

// NewMailer 创建邮件发送器（cfg.Enabled=false 时所有通知为 no-op）。
func NewMailer(db *gorm.DB, cfg config.Email, log *zap.Logger) *Mailer {
	return &Mailer{db: db, cfg: cfg, log: log}
}

// NotifyArticlePublished 通知发布者文章已发布。当前 goroutine 立即返回，
// 实际发送在后台进行；查邮箱、组信、发送每一步失败都只记日志。
func (m *Mailer) NotifyArticlePublished(publisherID uint, title, slug string, publishedAt time.Time) {
	if !m.cfg.Enabled {
		m.log.Debug("邮件通知未启用，跳过", zap.Uint("publisher_id", publisherID), zap.String("slug", slug))
		return
	}
	go func() {
		// WithoutCancel：发布请求结束后仍完整发送，仅受 10s 超时约束
		ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), sendTimeout)
		defer cancel()

		var u model.AdminUser
		if err := m.db.WithContext(ctx).First(&u, publisherID).Error; err != nil {
			m.log.Error("查询发布者邮箱失败", zap.Uint("publisher_id", publisherID), zap.Error(err))
			return
		}
		to := strings.TrimSpace(u.Email)
		if to == "" {
			m.log.Info("发布者未绑定邮箱，跳过邮件通知",
				zap.Uint("publisher_id", publisherID), zap.String("username", u.Username))
			return
		}

		subject := fmt.Sprintf("文章已发布：《%s》", title)
		articleURL := strings.TrimRight(m.cfg.SiteURL, "/") + "/article/" + slug
		body := publishedHTML(title, articleURL, publishedAt)
		if err := m.send(ctx, to, subject, body); err != nil {
			m.log.Error("发布通知邮件发送失败",
				zap.Uint("publisher_id", publisherID), zap.String("to", to), zap.String("slug", slug), zap.Error(err))
			return
		}
		m.log.Info("发布通知邮件已发送",
			zap.Uint("publisher_id", publisherID), zap.String("to", to), zap.String("slug", slug))
	}()
}

// send 通过 SMTP（SSL + SMTP AUTH）发送 HTML 邮件。
func (m *Mailer) send(ctx context.Context, to, subject, htmlBody string) error {
	msg := mail.NewMsg()
	if err := msg.EnvelopeFrom(m.cfg.From); err != nil {
		return fmt.Errorf("设置发件人: %w", err)
	}
	if err := msg.To(to); err != nil {
		return fmt.Errorf("设置收件人: %w", err)
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextHTML, htmlBody) // v0.8：无返回值，错误记录在 Msg 内部
	client, err := mail.NewClient(m.cfg.Host,
		mail.WithPort(m.cfg.Port),
		mail.WithSSL(),
		mail.WithSMTPAuth(mail.SMTPAuthPlain),
		mail.WithUsername(m.cfg.Username),
		mail.WithPassword(m.cfg.Password),
		mail.WithTimeout(sendTimeout),
	)
	if err != nil {
		return fmt.Errorf("创建 SMTP 客户端: %w", err)
	}
	if err := client.DialAndSendWithContext(ctx, msg); err != nil {
		return fmt.Errorf("SMTP 发送: %w", err)
	}
	return nil
}

// publishedHTML 组装发布通知邮件的 HTML 正文。
func publishedHTML(title, articleURL string, publishedAt time.Time) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><body style="margin:0;padding:24px;background:#f5f6f7;">`)
	b.WriteString(`<div style="max-width:520px;margin:0 auto;background:#fff;border:1px solid #e5e7eb;border-radius:12px;padding:28px 32px;">`)
	b.WriteString(`<p style="margin:0 0 16px;color:#6b7280;font-size:13px;">你好，你的博客文章已成功发布：</p>`)
	b.WriteString(`<p style="margin:0 0 8px;font-size:17px;font-weight:600;color:#111827;">`)
	fmt.Fprintf(&b, `<a href=%q style="color:#2563eb;text-decoration:none;">%s</a>`, articleURL, html.EscapeString(title))
	b.WriteString(`</p>`)
	b.WriteString(`<p style="margin:0 0 20px;color:#9ca3af;font-size:13px;">`)
	fmt.Fprintf(&b, `发布时间：%s · <a href=%q style="color:#2563eb;">阅读全文</a>`, publishedAt.Format("2006-01-02 15:04"), articleURL)
	b.WriteString(`</p>`)
	b.WriteString(`<hr style="border:none;border-top:1px solid #f0f0f0;margin:0 0 16px;" />`)
	b.WriteString(`<p style="margin:0;color:#9ca3af;font-size:12px;">AI 博客平台 · 系统邮件，请勿回复</p>`)
	b.WriteString(`</div></body></html>`)
	return b.String()
}
