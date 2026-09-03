package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"path/filepath"
	"strings"
	"time"
)

type Email struct{}

func NewEmail() *Email { return &Email{} }

type Attachment struct {
	Name string
	Data []byte
}

func (e *Email) Send(ctx context.Context, cfg EmailConfig, subject, body string) error {
	return e.SendWithAttachments(ctx, cfg, subject, body, nil)
}

// SendHTML — multipart/alternative (text+html) + optional related inline (CID logo).
func (e *Email) SendHTML(ctx context.Context, cfg EmailConfig, subject, textBody, htmlBody string, inline []InlineImage) error {
	host := strings.TrimSpace(cfg.SMTPHost)
	if host == "" {
		return fmt.Errorf("email: empty smtp_host")
	}
	port := cfg.SMTPPort
	if port <= 0 {
		port = 587
	}
	from := headerLineValue(cfg.From)
	if from == "" {
		return fmt.Errorf("email: empty from")
	}
	to := splitCSV(headerLineValue(cfg.To))
	if len(to) == 0 {
		return fmt.Errorf("email: empty recipients")
	}
	subject = headerLineValue(subject)
	if subject == "" {
		subject = "NetLynx Оповещение"
	}
	msg, err := buildHTMLMessage(from, to, subject, textBody, htmlBody, inline)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		done <- sendSMTP(cfg, host, port, from, to, msg)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

type InlineImage struct {
	CID         string // без < >, например netlynx-logo
	ContentType string
	Data        []byte
}

func (e *Email) SendWithAttachments(ctx context.Context, cfg EmailConfig, subject, body string, files []Attachment) error {
	host := strings.TrimSpace(cfg.SMTPHost)
	if host == "" {
		return fmt.Errorf("email: empty smtp_host")
	}
	port := cfg.SMTPPort
	if port <= 0 {
		port = 587
	}
	from := headerLineValue(cfg.From)
	if from == "" {
		return fmt.Errorf("email: empty from")
	}
	to := splitCSV(headerLineValue(cfg.To))
	if len(to) == 0 {
		return fmt.Errorf("email: empty recipients")
	}
	subject = headerLineValue(subject)
	if subject == "" {
		subject = "NetLynx alert"
	}

	var msg []byte
	var err error
	if len(files) == 0 {
		msg = []byte("From: " + from + "\r\n" +
			"To: " + strings.Join(to, ", ") + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n" +
			"\r\n" + body + "\r\n")
	} else {
		msg, err = buildMultipart(from, to, subject, body, files)
		if err != nil {
			return err
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- sendSMTP(cfg, host, port, from, to, msg)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func sendSMTP(cfg EmailConfig, host string, port int, from string, to []string, msg []byte) error {
	tlsCfg := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: cfg.TLSSkipVerify, //nolint:gosec // опция для внутренних SMTP по IP / self-signed
	}

	// Exchange/МФУ часто слушают 465 как plain + STARTTLS (не SMTPS).
	// Сначала submission-стиль; на 465 при провале — fallback на implicit TLS.
	c, err := dialSMTPClient(host, port, tlsCfg, false)
	if err != nil && port == 465 {
		c, err = dialSMTPClient(host, port, tlsCfg, true)
	}
	if err != nil {
		return err
	}
	defer c.Close()

	user := strings.TrimSpace(cfg.SMTPUsername)
	pass := strings.TrimSpace(cfg.SMTPPassword)
	if user != "" {
		auth := pickSMTPAuth(c, user, pass, host)
		if auth == nil {
			return fmt.Errorf("smtp: сервер не предлагает AUTH LOGIN/PLAIN")
		}
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func dialSMTPClient(host string, port int, tlsCfg *tls.Config, implicitTLS bool) (*smtp.Client, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	dialer := net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	var c *smtp.Client
	if implicitTLS {
		tlsConn := tls.Client(conn, tlsCfg)
		if err := tlsConn.Handshake(); err != nil {
			_ = conn.Close()
			return nil, err
		}
		c, err = smtp.NewClient(tlsConn, host)
	} else {
		c, err = smtp.NewClient(conn, host)
	}
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if !implicitTLS {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsCfg); err != nil {
				_ = c.Close()
				return nil, err
			}
		}
	}
	return c, nil
}

// pickSMTPAuth: Exchange часто даёт только LOGIN/NTLM (без PLAIN) → AUTH LOGIN.
func pickSMTPAuth(c *smtp.Client, user, pass, host string) smtp.Auth {
	ok, mechs := c.Extension("AUTH")
	upper := strings.ToUpper(mechs)
	if !ok || mechs == "" {
		return loginAuth{user, pass}
	}
	if strings.Contains(upper, "LOGIN") {
		return loginAuth{user, pass}
	}
	if strings.Contains(upper, "PLAIN") {
		return smtp.PlainAuth("", user, pass, host)
	}
	return loginAuth{user, pass}
}

// loginAuth — AUTH LOGIN (Microsoft Exchange / многие МФУ).
type loginAuth struct {
	username, password string
}

func (a loginAuth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	prompt := strings.ToLower(strings.TrimSpace(string(fromServer)))
	switch {
	case strings.Contains(prompt, "username"):
		return []byte(a.username), nil
	case strings.Contains(prompt, "password"):
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("smtp LOGIN: unexpected challenge %q", string(fromServer))
	}
}

func buildMultipart(from string, to []string, subject, body string, files []Attachment) ([]byte, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	hdr := "From: " + from + "\r\n" +
		"To: " + strings.Join(to, ", ") + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=" + w.Boundary() + "\r\n\r\n"
	if _, err := io.WriteString(&buf, hdr); err != nil {
		return nil, err
	}
	tw, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Type": {"text/plain; charset=UTF-8"},
	})
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(tw, body+"\r\n"); err != nil {
		return nil, err
	}
	for _, f := range files {
		name := headerLineValue(filepath.Base(strings.TrimSpace(f.Name)))
		if name == "" || name == "." {
			name = "backup.zip"
		}
		pw, err := w.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {"application/zip"},
			"Content-Transfer-Encoding": {"base64"},
			"Content-Disposition":       {fmt.Sprintf(`attachment; filename="%s"`, name)},
		})
		if err != nil {
			return nil, err
		}
		enc := base64.NewEncoder(base64.StdEncoding, pw)
		if _, err := enc.Write(f.Data); err != nil {
			_ = enc.Close()
			return nil, err
		}
		if err := enc.Close(); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildHTMLMessage(from string, to []string, subject, textBody, htmlBody string, inline []InlineImage) ([]byte, error) {
	var buf bytes.Buffer
	related := multipart.NewWriter(&buf)
	hdr := "From: " + from + "\r\n" +
		"To: " + strings.Join(to, ", ") + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/related; boundary=" + related.Boundary() + "\r\n\r\n"
	if _, err := io.WriteString(&buf, hdr); err != nil {
		return nil, err
	}

	altBuf := &bytes.Buffer{}
	alt := multipart.NewWriter(altBuf)
	tw, err := alt.CreatePart(textproto.MIMEHeader{
		"Content-Type": {"text/plain; charset=UTF-8"},
	})
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(tw, textBody); err != nil {
		return nil, err
	}
	hw, err := alt.CreatePart(textproto.MIMEHeader{
		"Content-Type": {"text/html; charset=UTF-8"},
	})
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(hw, htmlBody); err != nil {
		return nil, err
	}
	if err := alt.Close(); err != nil {
		return nil, err
	}

	aw, err := related.CreatePart(textproto.MIMEHeader{
		"Content-Type": {"multipart/alternative; boundary=" + alt.Boundary()},
	})
	if err != nil {
		return nil, err
	}
	if _, err := aw.Write(altBuf.Bytes()); err != nil {
		return nil, err
	}

	for _, img := range inline {
		cid := strings.TrimSpace(img.CID)
		if cid == "" || len(img.Data) == 0 {
			continue
		}
		ct := strings.TrimSpace(img.ContentType)
		if ct == "" {
			ct = "image/png"
		}
		pw, err := related.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {ct},
			"Content-Transfer-Encoding": {"base64"},
			"Content-ID":                {"<" + cid + ">"},
			"Content-Disposition":       {"inline"},
		})
		if err != nil {
			return nil, err
		}
		enc := base64.NewEncoder(base64.StdEncoding, pw)
		if _, err := enc.Write(img.Data); err != nil {
			_ = enc.Close()
			return nil, err
		}
		if err := enc.Close(); err != nil {
			return nil, err
		}
	}
	if err := related.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type EmailConfig struct {
	From          string
	To            string
	SMTPHost      string
	SMTPPort      int
	SMTPUsername  string
	SMTPPassword  string
	TLSSkipVerify bool // для SMTP по IP / self-signed
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		v := strings.TrimSpace(p)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func headerLineValue(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}
