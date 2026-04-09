package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	netmail "net/mail"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func LoadTokenCache() (*oauth2.Token, error) {
	const filePath = ".data/token_cache.json"
	if err := os.MkdirAll(".data", 0700); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, errors.New("token cache file is empty")
	}
	var tok oauth2.Token
	if err := json.Unmarshal(b, &tok); err != nil {
		return nil, fmt.Errorf("invalid token cache")
	}
	if strings.TrimSpace(tok.RefreshToken) == "" {
		return nil, errors.New("no refresh token in cache")
	}
	if tok.Expiry.Before(time.Now()) {
		return nil, errors.New("token is expired")
	}
	return &tok, nil
}

func WriteTokenCache(token *oauth2.Token) error {
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(".data/token_cache.json", data, 0600)
}

func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}

	return exec.Command(cmd, args...).Start()
}

func GetInitialToken(clientID, clientSecret string, localhost string) (*oauth2.Token, error) {
	cached, err := LoadTokenCache()
	if err == nil {
		refreshed, err := GetTokenViaLoopback(*cached, clientID, clientSecret)
		if err == nil {
			return &refreshed, nil
		}
	}

	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  "http://localhost:" + localhost + "/auth/callback",
		Scopes: []string{
			gmail.GmailModifyScope,
		},
		Endpoint: google.Endpoint,
	}

	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    ":" + localhost,
		Handler: mux,
	}

	var token *oauth2.Token
	var tokenErr error
	done := make(chan struct{})
	var once sync.Once
	closeDone := func() {
		once.Do(func() {
			close(done)
		})
	}

	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			tokenErr = fmt.Errorf("missing code parameter")
			http.Error(w, "missing code parameter", http.StatusBadRequest)
			closeDone()
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var err error
		token, err = config.Exchange(ctx, code)
		if err != nil {
			tokenErr = err
			http.Error(w, "failed to exchange token", http.StatusInternalServerError)
			closeDone()
			return
		}

		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><script>window.close();</script></body></html>`)
		closeDone()
	})

	listener, err := net.Listen("tcp", ":"+localhost)
	if err != nil {
		return nil, err
	}

	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			tokenErr = serveErr
			closeDone()
		}
	}()

	authURL := config.AuthCodeURL("state", oauth2.AccessTypeOffline)
	if err := openBrowser(authURL); err != nil {
		_ = server.Shutdown(context.Background())
		return nil, err
	}

	select {
	case <-done:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	case <-time.After(2 * time.Minute):
		_ = server.Shutdown(context.Background())
		return nil, fmt.Errorf("timeout waiting for OAuth callback")
	}

	if tokenErr != nil {
		return nil, tokenErr
	}

	err = WriteTokenCache(token)
	return token, err
}

type Mail struct {
	ID       string
	ThreadID string
	Subject  string
	From     string
	To       string
	Date     time.Time
	Text     string
	HTML     string
}

type EmailRecord struct {
	EmailID  string
	Subject  string
	From     string
	To       string
	Date     time.Time
	Snippet  string
	Contents string
}

func GetTokenViaLoopback(token oauth2.Token, clientID string, clientSecret string) (oauth2.Token, error) {
	if time.Now().Before(token.Expiry.Add(-time.Minute)) {
		return token, nil
	}
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("refresh_token", token.RefreshToken)
	data.Set("grant_type", "refresh_token")

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return token, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return token, errors.New(strconv.Itoa(resp.StatusCode))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return token, err
	}

	accessToken, ok := result["access_token"].(string)
	if !ok {
		return token, errors.New("access_token missing in refresh response")
	}
	token.AccessToken = accessToken

	expiresIn, ok := result["expires_in"].(float64)
	if !ok {
		return token, errors.New("expires_in missing in refresh response")
	}
	token.ExpiresIn = int64(expiresIn)
	token.Expiry = time.Now().Add(time.Duration(expiresIn) * time.Second)

	err = WriteTokenCache(&token)
	return token, err
}

type LoopbackSource struct {
	token        oauth2.Token
	clientID     string
	clientSecret string
}

func Make_Loopback_Source(token oauth2.Token, clientID, clientSecret string) LoopbackSource {
	return LoopbackSource{token: token, clientID: clientID, clientSecret: clientSecret}
}

func LoopbackRefresh(src LoopbackSource) (oauth2.Token, error) {
	t, err := GetTokenViaLoopback(src.token, src.clientID, src.clientSecret)
	src.token = t
	return src.token, err
}

func (src *LoopbackSource) Token() (*oauth2.Token, error) {
	token, err := GetTokenViaLoopback(src.token, src.clientID, src.clientSecret)
	src.token = token
	return &src.token, err
}

func NewGmailService(ctx context.Context, src LoopbackSource) (*gmail.Service, error) {
	token, err := LoopbackRefresh(src)
	if err != nil {
		return nil, err
	}
	ts := &LoopbackSource{token: token, clientID: src.clientID, clientSecret: src.clientSecret}
	return gmail.NewService(ctx, option.WithTokenSource(ts))
}

type Date struct {
	Year  int
	Month int
	Day   int
}

func Make_Date(year, month, day uint) Date {
	if day > 31 {
		fmt.Printf("Date cannot be greater than 31 or less than 0")
	}
	if month > 12 {
		fmt.Printf("Month cannot be greater than 12")
	}
	return Date{Year: int(year), Month: int(month), Day: int(day)}
}

func (date *Date) ToString() string {
	return strconv.Itoa(date.Year) + "/" + strconv.Itoa(date.Month) + "/" + strconv.Itoa(date.Day)
}

func FetchIDs(srv *gmail.Service, date Date) ([]string, error) {
	query := "after:" + date.ToString()
	var ids []string
	pageToken := ""

	for {
		result, err := srv.Users.Messages.List("me").Q(query).PageToken(pageToken).Do()
		if err != nil {
			return nil, err
		}

		for _, msg := range result.Messages {
			ids = append(ids, msg.Id)
		}

		if result.NextPageToken == "" {
			break
		}
		pageToken = result.NextPageToken
	}

	return ids, nil
}

func decodeBase64URL(data string) (string, error) {
	if strings.TrimSpace(data) == "" {
		return "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(data)
	if err == nil {
		return string(decoded), nil
	}
	decoded, err = base64.URLEncoding.DecodeString(data)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func decodePlainTextPart(part *gmail.MessagePart) (string, bool, error) {
	if part == nil {
		return "", false, nil
	}

	if part.MimeType == "text/plain" {
		text, err := decodeBase64URL(part.Body.Data)
		if err != nil {
			return "", false, err
		}
		if strings.TrimSpace(text) != "" {
			return text, true, nil
		}
	}

	for _, child := range part.Parts {
		text, ok, err := decodePlainTextPart(child)
		if err != nil {
			return "", false, err
		}
		if ok {
			return text, true, nil
		}
	}

	if part.Body != nil && strings.TrimSpace(part.Body.Data) != "" {
		text, err := decodeBase64URL(part.Body.Data)
		if err == nil && strings.TrimSpace(text) != "" {
			return text, true, nil
		}
	}

	return "", false, nil
}

func DecodeMessage(msg *gmail.Message) (string, error) {
	if msg == nil || msg.Payload == nil {
		return "", nil
	}
	text, ok, err := decodePlainTextPart(msg.Payload)
	if err != nil {
		return "", err
	}
	if ok {
		return text, nil
	}
	return "", nil
}

func headerValue(headers []*gmail.MessagePartHeader, name string) string {
	for _, h := range headers {
		if strings.EqualFold(h.Name, name) {
			return strings.TrimSpace(h.Value)
		}
	}
	return ""
}

func truncateRunes(value string, max int) string {
	trimmed := strings.TrimSpace(value)
	if max <= 0 || len(trimmed) == 0 {
		return ""
	}
	if utf8.RuneCountInString(trimmed) <= max {
		return trimmed
	}
	r := []rune(trimmed)
	return strings.TrimSpace(string(r[:max]))
}

func ExtractEmailRecord(msg *gmail.Message) (EmailRecord, error) {
	contents, err := DecodeMessage(msg)
	if err != nil {
		return EmailRecord{}, err
	}
	if msg == nil {
		return EmailRecord{}, nil
	}

	headers := []*gmail.MessagePartHeader{}
	if msg.Payload != nil {
		headers = msg.Payload.Headers
	}
	subject := headerValue(headers, "Subject")
	from := headerValue(headers, "From")
	to := headerValue(headers, "To")

	date := time.Time{}
	dateHeader := headerValue(headers, "Date")
	if strings.TrimSpace(dateHeader) != "" {
		if parsedDate, parseErr := netmail.ParseDate(dateHeader); parseErr == nil {
			date = parsedDate
		}
	}
	if date.IsZero() && msg.InternalDate > 0 {
		date = time.UnixMilli(msg.InternalDate)
	}
	if date.IsZero() {
		date = time.Now().UTC()
	}

	snippet := strings.TrimSpace(msg.Snippet)
	if snippet == "" {
		snippet = truncateRunes(contents, 180)
	}

	return EmailRecord{
		EmailID:  msg.Id,
		Subject:  subject,
		From:     from,
		To:       to,
		Date:     date,
		Snippet:  snippet,
		Contents: contents,
	}, nil
}

func FetchMessages(srv *gmail.Service, ids []string) ([]EmailRecord, error) {
	messages := make([]EmailRecord, 0, len(ids))

	for _, id := range ids {
		msg, err := srv.Users.Messages.Get("me", id).Format("full").Do()
		if err != nil {
			return nil, err
		}

		record, err := ExtractEmailRecord(msg)
		if err != nil {
			return nil, err
		}
		messages = append(messages, record)
	}

	return messages, nil
}
