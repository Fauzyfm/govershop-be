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

// SendAdminBalanceAlert sends an email to admin when Digiflazz balance is insufficient
func (s *Service) SendAdminBalanceAlert(toEmail string, data BalanceAlertData) error {
	from := s.config.SMTPFrom
	pass := s.config.SMTPPass
	host := s.config.SMTPHost
	port := s.config.SMTPPort

	auth := smtp.PlainAuth("", s.config.SMTPUser, pass, host)

	subject := "⚠️ ALERT: Saldo Digiflazz Kurang - Transaksi Gagal"

	customerEmailInfo := "-"
	if data.CustomerEmail != "" {
		customerEmailInfo = data.CustomerEmail
	}

	body := fmt.Sprintf(`
		<html>
		<body style="font-family: Arial, sans-serif; color: #333;">
			<h2 style="color: #e74c3c;">⚠️ Saldo Digiflazz Tidak Mencukupi</h2>
			<p>Ada customer yang <strong>gagal melakukan transaksi</strong> karena saldo Digiflazz tidak mencukupi.</p>
			
			<table style="border-collapse: collapse; width: 100%%; max-width: 500px;">
				<tr>
					<td style="padding: 8px; border: 1px solid #ddd; font-weight: bold;">Tanggal</td>
					<td style="padding: 8px; border: 1px solid #ddd;">%s</td>
				</tr>
				<tr>
					<td style="padding: 8px; border: 1px solid #ddd; font-weight: bold;">Jam</td>
					<td style="padding: 8px; border: 1px solid #ddd;">%s</td>
				</tr>
				<tr>
					<td style="padding: 8px; border: 1px solid #ddd; font-weight: bold;">Produk</td>
					<td style="padding: 8px; border: 1px solid #ddd;">%s (%s)</td>
				</tr>
				<tr>
					<td style="padding: 8px; border: 1px solid #ddd; font-weight: bold;">No Telepon Customer</td>
					<td style="padding: 8px; border: 1px solid #ddd;">%s</td>
				</tr>
				<tr>
					<td style="padding: 8px; border: 1px solid #ddd; font-weight: bold;">Email Customer</td>
					<td style="padding: 8px; border: 1px solid #ddd;">%s</td>
				</tr>
				<tr>
					<td style="padding: 8px; border: 1px solid #ddd; font-weight: bold;">Harga Beli (Modal)</td>
					<td style="padding: 8px; border: 1px solid #ddd;">Rp %s</td>
				</tr>
				<tr>
					<td style="padding: 8px; border: 1px solid #ddd; font-weight: bold;">Saldo Digiflazz</td>
					<td style="padding: 8px; border: 1px solid #ddd; color: #e74c3c;">Rp %s</td>
				</tr>
				<tr style="background-color: #ffeaa7;">
					<td style="padding: 8px; border: 1px solid #ddd; font-weight: bold;">Kekurangan Saldo</td>
					<td style="padding: 8px; border: 1px solid #ddd; color: #e74c3c; font-weight: bold;">Rp %s</td>
				</tr>
			</table>

			<p style="margin-top: 20px; color: #666;">Segera top-up saldo Digiflazz agar customer bisa melakukan transaksi.</p>
			<hr>
			<p style="font-size: 12px; color: #999;">Email otomatis dari sistem Govershop</p>
		</body>
		</html>
	`, data.Date, data.Time, data.ProductName, data.ProductSKU, data.CustomerPhone, customerEmailInfo,
		formatRupiah(data.BuyPrice), formatRupiah(data.CurrentBalance), formatRupiah(data.Deficit))

	msg := []byte("To: " + toEmail + "\r\n" +
		"From: " + from + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\"\r\n" +
		"\r\n" +
		body)

	addr := fmt.Sprintf("%s:%d", host, port)

	if err := smtp.SendMail(addr, auth, s.config.SMTPUser, []string{toEmail}, msg); err != nil {
		return fmt.Errorf("failed to send admin alert email: %w", err)
	}

	return nil
}

// formatRupiah formats a float64 as Indonesian Rupiah string (no decimals)
func formatRupiah(amount float64) string {
	// Simple formatting with thousand separators
	intAmount := int64(amount)
	if intAmount == 0 {
		return "0"
	}

	negative := false
	if intAmount < 0 {
		negative = true
		intAmount = -intAmount
	}

	// Convert to string with dots as thousand separator
	str := fmt.Sprintf("%d", intAmount)
	n := len(str)
	if n <= 3 {
		if negative {
			return "-" + str
		}
		return str
	}

	// Insert dots
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
