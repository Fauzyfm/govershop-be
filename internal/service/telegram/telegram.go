package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"govershop-api/internal/config"
	"govershop-api/internal/model"
)

// Service handles Telegram bot notifications
type Service struct {
	botToken string
	chatID   string
	enabled  bool
}

// NewService creates a new Telegram notification service
func NewService(cfg *config.Config) *Service {
	enabled := cfg.TelegramBotToken != "" && cfg.TelegramChatID != ""
	if enabled {
		log.Printf("📱 Telegram notification service initialized (chat_id: %s)", cfg.TelegramChatID)
	} else {
		log.Println("📱 Telegram notification service disabled (no token/chat_id configured)")
	}
	return &Service{
		botToken: cfg.TelegramBotToken,
		chatID:   cfg.TelegramChatID,
		enabled:  enabled,
	}
}

// IsConfigured returns true if Telegram bot is properly configured
func (s *Service) IsConfigured() bool {
	return s.enabled
}

// sendMessage sends a message via Telegram Bot API
func (s *Service) sendMessage(text string) error {
	if !s.enabled {
		return nil
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.botToken)

	payload := map[string]interface{}{
		"chat_id":    s.chatID,
		"text":       text,
		"parse_mode": "HTML",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to send telegram message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}

	return nil
}

// formatRupiah formats a float64 as Indonesian Rupiah string
func formatRupiah(amount float64) string {
	intAmount := int64(amount)
	if intAmount == 0 {
		return "Rp 0"
	}

	str := fmt.Sprintf("%d", intAmount)
	n := len(str)
	if n <= 3 {
		return "Rp " + str
	}

	var result []byte
	for i, c := range str {
		if i > 0 && (n-i)%3 == 0 {
			result = append(result, '.')
		}
		result = append(result, byte(c))
	}
	return "Rp " + string(result)
}

// NotifyOrderCreated sends notification when a new order is created (pending payment)
func (s *Service) NotifyOrderCreated(order *model.Order) {
	if !s.enabled {
		return
	}

	source := "🌐 Website"
	if order.OrderSource == "member" {
		source = "👤 Member"
	} else if order.OrderSource == "admin_cash" || order.OrderSource == "admin_gift" {
		source = "🔧 Admin (" + order.OrderSource + ")"
	}

	msg := fmt.Sprintf(
		"🆕 <b>ORDER BARU</b>\n"+
			"━━━━━━━━━━━━━━━\n"+
			"📦 <b>%s</b>\n"+
			"🔢 SKU: <code>%s</code>\n"+
			"📱 Tujuan: <code>%s</code>\n"+
			"💰 Harga: <b>%s</b>\n"+
			"📋 Status: <b>%s</b>\n"+
			"🏷️ Ref: <code>%s</code>\n"+
			"👤 Sumber: %s\n"+
			"🕐 %s",
		order.ProductName,
		order.BuyerSKUCode,
		order.CustomerNo,
		formatRupiah(order.SellingPrice),
		order.GetStatusLabel(),
		order.RefID,
		source,
		time.Now().Format("02 Jan 2006 15:04 WIB"),
	)

	if err := s.sendMessage(msg); err != nil {
		log.Printf("[Telegram] Failed to send order created notification: %v", err)
	}
}

// NotifyPaymentReceived sends notification when payment is confirmed
func (s *Service) NotifyPaymentReceived(order *model.Order) {
	if !s.enabled {
		return
	}

	msg := fmt.Sprintf(
		"💰 <b>PEMBAYARAN BERHASIL</b>\n"+
			"━━━━━━━━━━━━━━━\n"+
			"📦 <b>%s</b>\n"+
			"📱 Tujuan: <code>%s</code>\n"+
			"💰 Harga: <b>%s</b>\n"+
			"🏷️ Ref: <code>%s</code>\n"+
			"📋 Status: <b>Dibayar → Proses Topup</b>\n"+
			"🕐 %s",
		order.ProductName,
		order.CustomerNo,
		formatRupiah(order.SellingPrice),
		order.RefID,
		time.Now().Format("02 Jan 2006 15:04 WIB"),
	)

	if err := s.sendMessage(msg); err != nil {
		log.Printf("[Telegram] Failed to send payment notification: %v", err)
	}
}

// NotifyTopupResult sends notification with Digiflazz topup result
func (s *Service) NotifyTopupResult(order *model.Order, digiflazzStatus, rc, sn, message string) {
	if !s.enabled {
		return
	}

	var statusIcon, statusLabel string
	switch digiflazzStatus {
	case "Sukses":
		statusIcon = "✅"
		statusLabel = "SUKSES"
	case "Gagal":
		statusIcon = "❌"
		statusLabel = "GAGAL"
	default:
		statusIcon = "⏳"
		statusLabel = "PENDING"
	}

	msg := fmt.Sprintf(
		"%s <b>TOPUP %s</b>\n"+
			"━━━━━━━━━━━━━━━\n"+
			"📦 <b>%s</b>\n"+
			"📱 Tujuan: <code>%s</code>\n"+
			"💰 Harga: <b>%s</b>\n"+
			"🏷️ Ref: <code>%s</code>\n",
		statusIcon,
		statusLabel,
		order.ProductName,
		order.CustomerNo,
		formatRupiah(order.SellingPrice),
		order.RefID,
	)

	if sn != "" {
		msg += fmt.Sprintf("🔑 SN: <code>%s</code>\n", sn)
	}
	if rc != "" {
		msg += fmt.Sprintf("📊 RC: <code>%s</code>\n", rc)
	}
	if message != "" {
		msg += fmt.Sprintf("💬 Pesan: %s\n", message)
	}

	// Add member info if applicable
	if order.MemberID != nil {
		msg += fmt.Sprintf("👤 Member ID: %d\n", *order.MemberID)
		if digiflazzStatus == "Gagal" {
			msg += "💸 <i>Saldo member di-refund otomatis</i>\n"
		}
	}

	msg += fmt.Sprintf("🕐 %s", time.Now().Format("02 Jan 2006 15:04 WIB"))

	if err := s.sendMessage(msg); err != nil {
		log.Printf("[Telegram] Failed to send topup result notification: %v", err)
	}
}
