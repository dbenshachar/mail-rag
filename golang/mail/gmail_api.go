package mail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

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

	err := exec.Command(cmd, args...).Start()
	return err
}

func LoadToken(api_config_path string) {

}

func GetInitialToken(clientID, clientSecret string, localhost string) (*oauth2.Token, error) {
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  "http://localhost:" + localhost + "/auth/callback",
		Scopes: []string{
			gmail.GmailModifyScope,
		},
		Endpoint: google.Endpoint,
	}

	server := &http.Server{
		Addr: ":" + localhost,
	}

	var token *oauth2.Token
	var tokenErr error
	done := make(chan struct{})

	http.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			tokenErr = fmt.Errorf("missing code parameter")
			http.Error(w, "missing code parameter", http.StatusBadRequest)
			close(done)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var err error
		token, err = config.Exchange(ctx, code)
		if err != nil {
			tokenErr = err
			http.Error(w, "failed to exchange token", http.StatusInternalServerError)
			close(done)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><script>window.close();</script></body></html>`)
		close(done)
	})

	listener, err := net.Listen("tcp", ":"+localhost)
	if err != nil {
		return nil, err
	}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	authURL := config.AuthCodeURL("state", oauth2.AccessTypeOffline)
	openBrowser(authURL)

	select {
	case <-done:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	case <-time.After(30 * time.Second):
		server.Shutdown(context.Background())
		return nil, fmt.Errorf("timeout waiting for OAuth callback")
	}

	if tokenErr != nil {
		return nil, tokenErr
	}

	return token, nil
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

	if accessToken, ok := result["access_token"].(string); ok {
		token.AccessToken = accessToken
	} else {
		return token, err
	}

	if expiresIn, ok := result["expires_in"].(float64); ok {
		token.ExpiresIn = int64(expiresIn)
		return token, err
	}

	return token, nil
}

type loopbackSource struct {
	token        oauth2.Token
	clientID     string
	clientSecret string
}

func Make_Loopback_Source(token oauth2.Token, clientID, clientSecret string) loopbackSource {
	return loopbackSource{token: token, clientID: clientID, clientSecret: clientSecret}
}

func LoopbackRefresh(src loopbackSource) (oauth2.Token, error) {
	t, err := GetTokenViaLoopback(src.token, src.clientID, src.clientSecret)
	src.token = t
	return src.token, err
}

func (src *loopbackSource) Token() (*oauth2.Token, error) {
	token, err := GetTokenViaLoopback(src.token, src.clientID, src.clientSecret)
	src.token = token
	return &src.token, err
}

func NewGmailService(ctx context.Context, src loopbackSource) (*gmail.Service, error) {
	token, _ := LoopbackRefresh(src)
	ts := &loopbackSource{token: token, clientID: src.clientID, clientSecret: src.clientSecret}
	srv, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	return srv, err
}

type Date struct {
	year  int
	month int
	day   int
}

func Make_Date(year, month, day uint) Date {
	if day > 31 {
		fmt.Printf("Date cannot be greater than 31 or less than 0")
	}
	if month > 12 {
		fmt.Printf("Month cannot be greater than 12")
	}
	return Date{year: int(year), month: int(month), day: int(day)}
}

func (date *Date) ToString() string {
	return strconv.Itoa(date.year) + "/" + strconv.Itoa(date.month) + "/" + strconv.Itoa(date.day)
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

func DecodeMessage(msg *gmail.Message) (string, error) {
	var body string

	if msg.Payload.Parts == nil {
		body = msg.Payload.Body.Data
	} else {
		for _, part := range msg.Payload.Parts {
			if part.MimeType == "text/plain" {
				body = part.Body.Data
				break
			}
		}
	}

	decoded, err := base64.URLEncoding.DecodeString(body)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}

func FetchMessages(srv *gmail.Service, ids []string) ([]string, error) {
	var messages []string

	for _, id := range ids {
		msg, err := srv.Users.Messages.Get("me", id).Do()
		if err != nil {
			return nil, err
		}

		content, err := DecodeMessage(msg)
		if err != nil {
			return nil, err
		}
		messages = append(messages, content)
	}

	return messages, nil
}
