package notification

// cov_telegram_test.go 覆盖 telegram 渠道 Send 的成功返回（2xx）：
// 通过自定义 RoundTripper 拦截对 api.telegram.org 的请求，不触网。

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/config"
)

type covRTFunc func(*http.Request) (*http.Response, error)

func (f covRTFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCovTelegramSendSuccess(t *testing.T) {
	var gotURL, gotBody string
	ch := &telegramChannel{
		cfg: &config.TelegramConfig{BotToken: "cov-token", ChatID: "42"},
		client: &http.Client{Transport: covRTFunc(func(r *http.Request) (*http.Response, error) {
			gotURL = r.URL.String()
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			}, nil
		})},
	}

	if err := ch.Send(context.Background(), &Notification{Title: "t", Message: "m"}); err != nil {
		t.Fatalf("Send err = %v", err)
	}
	if gotURL != "https://api.telegram.org/botcov-token/sendMessage" {
		t.Errorf("url = %s", gotURL)
	}
	if !strings.Contains(gotBody, `"chat_id":"42"`) || !strings.Contains(gotBody, "HTML") {
		t.Errorf("payload = %s", gotBody)
	}
}

// 非 2xx 状态码视为发送失败（渠道侧错误原样带状态码返回）
func TestCovTelegramSendNon2xx(t *testing.T) {
	ch := &telegramChannel{
		cfg: &config.TelegramConfig{BotToken: "cov-token", ChatID: "42"},
		client: &http.Client{Transport: covRTFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Status:     "500 Internal Server Error",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{}`)),
			}, nil
		})},
	}

	err := ch.Send(context.Background(), &Notification{Title: "t", Message: "m"})
	if err == nil || !strings.Contains(err.Error(), "unexpected status code 500") {
		t.Fatalf("Send err = %v, want unexpected status code 500", err)
	}
}
