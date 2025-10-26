package golang

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

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
