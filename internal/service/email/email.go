package email

import (
	"fmt"
	"net/smtp"

	"govershop-api/internal/config"
)

type Service struct {
	config *config.Config
}

func NewService(cfg *config.Config) *Service {
	return &Service{config: cfg}
}

func (s *Service) SendResetPasswordEmail(toEmail, resetLink string) error {
	from := s.config.SMTPFrom
	pass := s.config.SMTPPass
	host := s.config.SMTPHost
	port := s.config.SMTPPort

	// Basic auth
	auth := smtp.PlainAuth("", s.config.SMTPUser, pass, host)

	// Email content
	subject := "Reset Password Govershop"
	body := fmt.Sprintf(`
		<html>
		<body>
			<h2>Reset Password</h2>
			<p>Anda membinta untuk mereset password akun Govershop Anda.</p>
			<p>Silakan klik link di bawah ini untuk mereset password:</p>
			<p><a href="%s">Reset Password</a></p>
			<p>Atau copy link ini: %s</p>
			<p>Link ini valid selama 1 jam.</p>
			<p>Jika Anda tidak meminta ini, abaikan saja.</p>
		</body>
		</html>
	`, resetLink, resetLink)

	msg := []byte("To: " + toEmail + "\r\n" +
		"From: " + from + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\"\r\n" +
		"\r\n" +
		body)

	addr := fmt.Sprintf("%s:%d", host, port)

	// Send email
	if err := smtp.SendMail(addr, auth, s.config.SMTPUser, []string{toEmail}, msg); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}
