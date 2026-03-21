package worker

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"time"

	"github.com/arnabmitra/eth-proxy/internal/repository"
)

type AlertWorker struct {
	repo           *repository.Queries
	logger         *slog.Logger
	interval       time.Duration
	stop           chan struct{}
	lastAlerted    map[string]time.Time
}

func NewAlertWorker(repo *repository.Queries, logger *slog.Logger) *AlertWorker {
	return &AlertWorker{
		repo:        repo,
		logger:      logger,
		interval:    15 * time.Minute,
		stop:        make(chan struct{}),
		lastAlerted: make(map[string]time.Time),
	}
}

func (w *AlertWorker) Start() {
	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		
		// Run check on start
		w.checkAlerts()

		for {
			select {
			case <-ticker.C:
				w.checkAlerts()
			case <-w.stop:
				return
			}
		}
	}()
}

func (w *AlertWorker) Stop() {
	close(w.stop)
}

func (w *AlertWorker) checkAlerts() {
	if !isMarketOpen() {
		return
	}

	ctx := context.Background()
	results, err := w.repo.GetLatestZScores(ctx)
	if err != nil {
		w.logger.Error("failed to get latest z-scores for alerts", "error", err)
		return
	}

	for _, r := range results {
		z, _ := r.ZScore.Float64Value()
		zScore := z.Float64

		// ALERT LOGIC: Extreme levels (Absolute Z > 2.5)
		if math.Abs(zScore) >= 2.5 {
			// Don't alert more than once every 4 hours for the same symbol
			if last, ok := w.lastAlerted[r.Symbol]; ok && time.Since(last) < 4*time.Hour {
				continue
			}

			message := fmt.Sprintf("⚠️ GEX ALERT: %s is at extreme deviation: %.2fσ. Current GEX: %.0f", 
				r.Symbol, zScore, r.GexValue)
			
			w.sendAlert(r.Symbol, message)
			w.lastAlerted[r.Symbol] = time.Now()
		}
	}
}

func (w *AlertWorker) sendAlert(symbol, message string) {
	w.logger.Info("sending alert", "symbol", symbol, "message", message)

	// Send to Telegram if configured
	go w.sendTelegram(message)

	// Send to Email if configured
	go w.sendEmail(message)
}

func (w *AlertWorker) sendTelegram(message string) {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")
	if botToken == "" || chatID == "" {
		return
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	formData := url.Values{
		"chat_id": {chatID},
		"text":    {message},
	}

	resp, err := http.PostForm(apiURL, formData)
	if err != nil {
		w.logger.Error("failed to send telegram alert", "error", err)
		return
	}
	defer resp.Body.Close()
}

func (w *AlertWorker) sendEmail(message string) {
	to := os.Getenv("ALERT_EMAIL")
	from := os.Getenv("SMTP_FROM")
	password := os.Getenv("SMTP_PASSWORD")
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")

	if to == "" || from == "" || password == "" || host == "" {
		return
	}

	auth := smtp.PlainAuth("", from, password, host)
	body := fmt.Sprintf("Subject: GEX Trading Alert\r\n\r\n%s", message)

	addr := fmt.Sprintf("%s:%s", host, port)
	err := smtp.SendMail(addr, auth, from, []string{to}, []byte(body))
	if err != nil {
		w.logger.Error("failed to send email alert", "error", err)
	}
}
