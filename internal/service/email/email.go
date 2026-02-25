package email

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/smtp"

	"govershop-api/internal/config"
)

//go:embed templates/*.html
var templateFS embed.FS

var templates *template.Template

func init() {
	templates = template.Must(template.ParseFS(templateFS, "templates/*.html"))
}

type Service struct {
	config *config.Config
}

func NewService(cfg *config.Config) *Service {
	return &Service{config: cfg}
}

// resetPasswordData is the data passed to reset_password.html
type resetPasswordData struct {
	ResetLink string
}

func (s *Service) SendResetPasswordEmail(toEmail, resetLink string) error {
	from := s.config.SMTPFrom
	pass := s.config.SMTPPass
	host := s.config.SMTPHost
	port := s.config.SMTPPort

	auth := smtp.PlainAuth("", s.config.SMTPUser, pass, host)

	subject := "Reset Password — Restopup"

	data := resetPasswordData{
		ResetLink: resetLink,
	}

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "reset_password.html", data); err != nil {
		return fmt.Errorf("failed to render reset password template: %w", err)
	}

	msg := []byte("To: " + toEmail + "\r\n" +
		"From: " + from + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\"\r\n" +
		"\r\n" +
		buf.String())

	addr := fmt.Sprintf("%s:%d", host, port)

	if err := smtp.SendMail(addr, auth, s.config.SMTPUser, []string{toEmail}, msg); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

// BalanceAlertData holds data for the admin balance alert email
type BalanceAlertData struct {
	Date           string // e.g. "20 Februari 2026"
	Time           string // e.g. "19:04 WIB"
	ProductName    string
	ProductSKU     string
	CustomerPhone  string
	CustomerEmail  string
	BuyPrice       float64 // Harga beli (modal) dari Digiflazz
	CurrentBalance float64
	Deficit        float64 // Kekurangan saldo
}

// balanceAlertTemplateData is the pre-formatted data passed to balance_alert.html
type balanceAlertTemplateData struct {
	Date           string
	Time           string
	ProductName    string
	ProductSKU     string
	CustomerPhone  string
	CustomerEmail  string
	BuyPrice       string
	CurrentBalance string
	Deficit        string
}

// SendAdminBalanceAlert sends an email to admin when Digiflazz balance is insufficient
func (s *Service) SendAdminBalanceAlert(toEmail string, data BalanceAlertData) error {
	from := s.config.SMTPFrom
	pass := s.config.SMTPPass
	host := s.config.SMTPHost
	port := s.config.SMTPPort

	auth := smtp.PlainAuth("", s.config.SMTPUser, pass, host)

	subject := "⚠️ ALERT: Saldo Digiflazz Kurang — Transaksi Gagal"

	customerEmailInfo := "-"
	if data.CustomerEmail != "" {
		customerEmailInfo = data.CustomerEmail
	}

	tplData := balanceAlertTemplateData{
		Date:           data.Date,
		Time:           data.Time,
		ProductName:    data.ProductName,
		ProductSKU:     data.ProductSKU,
		CustomerPhone:  data.CustomerPhone,
		CustomerEmail:  customerEmailInfo,
		BuyPrice:       formatRupiah(data.BuyPrice),
		CurrentBalance: formatRupiah(data.CurrentBalance),
		Deficit:        formatRupiah(data.Deficit),
	}

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "balance_alert.html", tplData); err != nil {
		return fmt.Errorf("failed to render balance alert template: %w", err)
	}

	msg := []byte("To: " + toEmail + "\r\n" +
		"From: " + from + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\"\r\n" +
		"\r\n" +
		buf.String())

	addr := fmt.Sprintf("%s:%d", host, port)

	if err := smtp.SendMail(addr, auth, s.config.SMTPUser, []string{toEmail}, msg); err != nil {
		return fmt.Errorf("failed to send admin alert email: %w", err)
	}

	return nil
}

// formatRupiah formats a float64 as Indonesian Rupiah string (no decimals)
func formatRupiah(amount float64) string {
	intAmount := int64(amount)
	if intAmount == 0 {
		return "0"
	}

	negative := false
	if intAmount < 0 {
		negative = true
		intAmount = -intAmount
	}

	str := fmt.Sprintf("%d", intAmount)
	n := len(str)
	if n <= 3 {
		if negative {
			return "-" + str
		}
		return str
	}

	var result []byte
	for i, c := range str {
		if i > 0 && (n-i)%3 == 0 {
			result = append(result, '.')
		}
		result = append(result, byte(c))
	}

	if negative {
		return "-" + string(result)
	}
	return string(result)
}
