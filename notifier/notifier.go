package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Notifier sends error alerts to Telegram with a cooldown to prevent spam.
type Notifier struct {
	botToken string
	chatID   string
	threadID string
	cooldown time.Duration

	mu       sync.Mutex
	lastSent time.Time
}

type telegramMessage struct {
	ChatID    string `json:"chat_id"`
	MessageThreadId string `json:"message_thread_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

func New(botToken, chatID, threadID string, cooldown time.Duration) *Notifier {
	return &Notifier{
		botToken: botToken,
		chatID:   chatID,
		threadID: threadID,
		cooldown: cooldown,
	}
}

// NotifyError logs the error always, and sends a Telegram message if:
//   - botToken and chatID are configured
//   - cooldown period has passed since last notification
func (n *Notifier) NotifyError(context string, err error) {
	slog.Error("watcher error", "context", context, "error", err)

	if n.botToken == "" || n.chatID == "" {
		// Telegram not configured yet — skip silently
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if time.Since(n.lastSent) < n.cooldown {
		slog.Debug("telegram cooldown active, skipping notification",
			"remaining", n.cooldown-time.Since(n.lastSent))
		return
	}

	msg := fmt.Sprintf("🚨 *solscan-watcher error*\n\n`%s`\n\n%s", context, err.Error())
	if sendErr := n.send(msg); sendErr != nil {
		slog.Error("failed to send telegram notification", "error", sendErr)
		return
	}

	n.lastSent = time.Now()
}

func (n *Notifier) send(text string) error {
	payload := telegramMessage{
		ChatID:    n.chatID,
		Text:      text,
		MessageThreadId : n.threadID,
		ParseMode: "Markdown",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telegram payload: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.botToken)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post telegram: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram returned status %d", resp.StatusCode)
	}

	return nil
}
