package wiki

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"golang.org/x/net/proxy"
	"html"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type DeliveryError struct{ Code string }

func (e *DeliveryError) Error() string { return "provider acceptance failed" }
func deliveryResult(err error) string {
	if err == nil {
		return "accepted"
	}
	var p *DeliveryError
	if errors.As(err, &p) && regexp.MustCompile(`^[A-Za-z0-9_.-]{1,80}$`).MatchString(p.Code) {
		return p.Code
	}
	return "unconfirmed"
}

type SMTPConfig struct {
	Host, User, Password, From, TLSMode, ProxyURL string
	Port                                          int
	ConnectTimeout, SendTimeout                   time.Duration
}
type SMTPSender struct{ config SMTPConfig }

func NewSMTPSender(c SMTPConfig) (*SMTPSender, error) {
	if c.Port == 0 {
		c.Port = 465
	}
	if c.TLSMode == "" {
		c.TLSMode = "implicit_tls"
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = 5 * time.Second
	}
	if c.SendTimeout == 0 {
		c.SendTimeout = 10 * time.Second
	}
	from, e := mail.ParseAddress(c.From)
	if e != nil || strings.ContainsAny(c.From+c.User, "\r\n") || from.Address != c.User || c.Host == "" || c.Password == "" || c.Port < 1 || c.Port > 65535 || c.TLSMode != "implicit_tls" && c.TLSMode != "starttls" {
		return nil, fmt.Errorf("valid SMTP server, matching sender/user, password and secure TLS mode are required")
	}
	return &SMTPSender{c}, nil
}
func mailCodeMessage(from, to, code, purpose string) ([]byte, error) {
	if strings.ContainsAny(from+to+code, "\r\n") {
		return nil, fmt.Errorf("invalid verification message")
	}
	names := map[string]string{"login": "登录", "join": "加入 Wiki", "bind": "绑定联系方式", "replace": "更换联系方式", "reauth": "身份验证", "recover": "账号找回"}
	action := names[purpose]
	if action == "" {
		return nil, fmt.Errorf("invalid verification purpose")
	}
	text := fmt.Sprintf("鸟托邦 Wiki\r\n您的%s验证码为：%s。5 分钟内有效，请勿告诉他人。\r\n如果不是您本人操作，请忽略本邮件。", action, code)
	var out bytes.Buffer
	multi := multipart.NewWriter(&out)
	fmt.Fprintf(&out, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%q\r\n\r\n", from, to, mime.QEncoding.Encode("UTF-8", "鸟托邦 Wiki 验证码"), multi.Boundary())
	for _, p := range []struct{ kind, body string }{{"text/plain", text}, {"text/html", "<h1>鸟托邦 Wiki</h1><p>" + html.EscapeString(action) + "验证码：</p><p><strong>" + html.EscapeString(code) + "</strong></p><p>5 分钟内有效，请勿告诉他人。非本人操作请忽略。</p>"}} {
		part, e := multi.CreatePart(map[string][]string{"Content-Type": {p.kind + "; charset=UTF-8"}, "Content-Transfer-Encoding": {"quoted-printable"}})
		if e != nil {
			return nil, e
		}
		q := quotedprintable.NewWriter(part)
		if _, e = q.Write([]byte(p.body)); e != nil {
			return nil, e
		}
		if e = q.Close(); e != nil {
			return nil, e
		}
	}
	if e := multi.Close(); e != nil {
		return nil, e
	}
	return out.Bytes(), nil
}
func (s *SMTPSender) Send(ctx context.Context, target, code, purpose, id string) error {
	c := s.config
	raw, e := mailCodeMessage(c.From, target, code, purpose)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(ctx, c.SendTimeout)
	defer cancel()
	dialer := net.Dialer{Timeout: c.ConnectTimeout}
	address := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	conn, e := dialSMTP(ctx, c, dialer, address)
	tlsConfig := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
	if e == nil && c.TLSMode == "implicit_tls" {
		wrapped := tls.Client(conn, tlsConfig)
		if e = wrapped.HandshakeContext(ctx); e != nil {
			conn.Close()
		} else {
			conn = wrapped
		}
	}

	if e != nil {
		return &DeliveryError{Code: "smtp_connection_or_tls"}
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	client, e := smtp.NewClient(conn, c.Host)
	if e != nil {
		return &DeliveryError{Code: "smtp_greeting_failed"}
	}
	defer client.Close()
	if c.TLSMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return &DeliveryError{Code: "smtp_tls_is_required"}
		}
		if e = client.StartTLS(tlsConfig); e != nil {
			return &DeliveryError{Code: "smtp_tls_failed"}
		}
	}
	if e = client.Auth(smtp.PlainAuth("", c.User, c.Password, c.Host)); e != nil {
		return &DeliveryError{Code: "smtp_authentication_failed"}
	}
	if e = client.Mail(c.User); e != nil {
		return &DeliveryError{Code: "smtp_sender_rejected"}
	}
	if e = client.Rcpt(target); e != nil {
		return &DeliveryError{Code: "smtp_recipient_rejected"}
	}
	writer, e := client.Data()
	if e != nil {
		return &DeliveryError{Code: "smtp_message_rejected"}
	}
	if _, e = writer.Write(raw); e != nil {
		return &DeliveryError{Code: "smtp_write_failed"}
	}
	if e = writer.Close(); e != nil {
		return &DeliveryError{Code: "smtp_acceptance_not_confirmed"}
	}
	_ = client.Quit()
	return nil
}

type bufferedTunnel struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedTunnel) Read(b []byte) (int, error) { return c.reader.Read(b) }
func dialSMTP(ctx context.Context, c SMTPConfig, dialer net.Dialer, address string) (net.Conn, error) {
	if c.ProxyURL == "" {
		return dialer.DialContext(ctx, "tcp", address)
	}
	u, e := url.Parse(c.ProxyURL)
	if e != nil {
		return nil, e
	}
	if u.Scheme == "socks5" || u.Scheme == "socks5h" {
		u.Scheme = "socks5"
		d, e := proxy.FromURL(u, &dialer)
		if e != nil {
			return nil, e
		}
		if d, ok := d.(proxy.ContextDialer); ok {
			return d.DialContext(ctx, "tcp", address)
		}
		return nil, fmt.Errorf("proxy does not support cancellation")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("SMTP proxy requires http, https or socks5")
	}
	proxyAddress := u.Host
	if u.Port() == "" {
		port := "80"
		if u.Scheme == "https" {
			port = "443"
		}
		proxyAddress = net.JoinHostPort(u.Hostname(), port)
	}
	conn, e := dialer.DialContext(ctx, "tcp", proxyAddress)
	if e != nil {
		return nil, e
	}
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	if u.Scheme == "https" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12})
		if e = tlsConn.HandshakeContext(ctx); e != nil {
			conn.Close()
			return nil, e
		}
		conn = tlsConn
	}
	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: address}, Host: address, Header: make(http.Header)}
	if u.User != nil {
		pass, _ := u.User.Password()
		req.SetBasicAuth(u.User.Username(), pass)
		req.Header.Set("Proxy-Authorization", req.Header.Get("Authorization"))
		req.Header.Del("Authorization")
	}
	if e = req.Write(conn); e != nil {
		conn.Close()
		return nil, e
	}
	reader := bufio.NewReader(conn)
	resp, e := http.ReadResponse(reader, req)
	if e != nil {
		conn.Close()
		return nil, e
	}
	if resp.StatusCode != 200 {
		conn.Close()
		return nil, fmt.Errorf("SMTP proxy refused tunnel")
	}
	return &bufferedTunnel{conn, reader}, nil
}
