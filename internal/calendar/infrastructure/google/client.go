package google

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"appointment-service/internal/calendar/domain"
)

var ErrNotFound = errors.New("google calendar event not found")

type Client struct {
	calendarID string
	baseURL    string
	httpClient *http.Client
	tokens     tokenProvider
}

func NewClient(
	calendarID string,
	baseURL string,
	accessToken string,
	serviceAccountEmail string,
	serviceAccountPrivateKey string,
	tokenURI string,
) *Client {
	var provider tokenProvider
	if strings.TrimSpace(accessToken) != "" {
		provider = &staticTokenProvider{accessToken: strings.TrimSpace(accessToken)}
	} else {
		provider = &serviceAccountTokenProvider{
			email:      strings.TrimSpace(serviceAccountEmail),
			privateKey: strings.TrimSpace(serviceAccountPrivateKey),
			tokenURI:   strings.TrimSpace(tokenURI),
			httpClient: &http.Client{Timeout: 10 * time.Second},
		}
	}

	return &Client{
		calendarID: strings.TrimSpace(calendarID),
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		tokens:     provider,
	}
}

func (c *Client) Enabled() bool {
	if c == nil || c.tokens == nil {
		return false
	}
	return c.calendarID != "" && c.tokens.Enabled()
}

func (c *Client) CreateEvent(ctx context.Context, payload domain.AppointmentEventPayload) (string, error) {
	body := googleEventRequest{
		Summary:     renderSummary(payload),
		Description: renderDescription(payload),
		Start: googleEventDateTime{
			DateTime: payload.StartTime.UTC().Format(time.RFC3339),
			TimeZone: "UTC",
		},
		End: googleEventDateTime{
			DateTime: payload.EndTime.UTC().Format(time.RFC3339),
			TimeZone: "UTC",
		},
	}

	respBody, status, err := c.request(ctx, http.MethodPost, c.eventsPath(""), body)
	if err != nil {
		return "", err
	}
	if status >= http.StatusBadRequest {
		return "", fmt.Errorf("google create event failed: status=%d body=%s", status, respBody)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decode google create event response: %w", err)
	}
	if strings.TrimSpace(result.ID) == "" {
		return "", fmt.Errorf("google create event response has empty id")
	}
	return result.ID, nil
}

func (c *Client) UpdateEvent(ctx context.Context, googleEventID string, payload domain.AppointmentEventPayload) error {
	body := googleEventRequest{
		Summary:     renderSummary(payload),
		Description: renderDescription(payload),
		Start: googleEventDateTime{
			DateTime: payload.StartTime.UTC().Format(time.RFC3339),
			TimeZone: "UTC",
		},
		End: googleEventDateTime{
			DateTime: payload.EndTime.UTC().Format(time.RFC3339),
			TimeZone: "UTC",
		},
	}

	respBody, status, err := c.request(ctx, http.MethodPatch, c.eventsPath(googleEventID), body)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return ErrNotFound
	}
	if status >= http.StatusBadRequest {
		return fmt.Errorf("google update event failed: status=%d body=%s", status, respBody)
	}
	return nil
}

func (c *Client) DeleteEvent(ctx context.Context, googleEventID string) error {
	respBody, status, err := c.request(ctx, http.MethodDelete, c.eventsPath(googleEventID), nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return ErrNotFound
	}
	if status >= http.StatusBadRequest {
		return fmt.Errorf("google delete event failed: status=%d body=%s", status, respBody)
	}
	return nil
}

func (c *Client) request(ctx context.Context, method string, path string, payload any) ([]byte, int, error) {
	if !c.Enabled() {
		return nil, 0, fmt.Errorf("google calendar client is disabled")
	}
	token, err := c.tokens.AccessToken(ctx)
	if err != nil {
		return nil, 0, err
	}

	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal google request body: %w", err)
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, 0, fmt.Errorf("build google request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("google request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return nil, 0, fmt.Errorf("read google response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

func (c *Client) eventsPath(eventID string) string {
	base := "/calendars/" + url.PathEscape(c.calendarID) + "/events"
	if strings.TrimSpace(eventID) == "" {
		return base
	}
	return base + "/" + url.PathEscape(eventID)
}

func renderSummary(payload domain.AppointmentEventPayload) string {
	return fmt.Sprintf("Appointment: %s with %s", payload.ClientName, payload.SpecialistName)
}

func renderDescription(payload domain.AppointmentEventPayload) string {
	return fmt.Sprintf(
		"appointment_id=%d\nevent_type=%s\nclient=%s\nspecialist=%s",
		payload.AppointmentID,
		payload.EventType,
		payload.ClientName,
		payload.SpecialistName,
	)
}

type googleEventRequest struct {
	Summary     string              `json:"summary"`
	Description string              `json:"description,omitempty"`
	Start       googleEventDateTime `json:"start"`
	End         googleEventDateTime `json:"end"`
}

type googleEventDateTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone,omitempty"`
}

type tokenProvider interface {
	Enabled() bool
	AccessToken(ctx context.Context) (string, error)
}

type staticTokenProvider struct {
	accessToken string
}

func (p *staticTokenProvider) Enabled() bool {
	return strings.TrimSpace(p.accessToken) != ""
}

func (p *staticTokenProvider) AccessToken(_ context.Context) (string, error) {
	if !p.Enabled() {
		return "", fmt.Errorf("GOOGLE_CALENDAR_ACCESS_TOKEN is empty")
	}
	return p.accessToken, nil
}

type serviceAccountTokenProvider struct {
	email      string
	privateKey string
	tokenURI   string
	httpClient *http.Client

	mu          sync.Mutex
	cachedToken string
	expiresAt   time.Time
}

func (p *serviceAccountTokenProvider) Enabled() bool {
	return p.email != "" && p.privateKey != "" && p.tokenURI != ""
}

func (p *serviceAccountTokenProvider) AccessToken(ctx context.Context) (string, error) {
	if !p.Enabled() {
		return "", fmt.Errorf("service account credentials are incomplete")
	}

	p.mu.Lock()
	if p.cachedToken != "" && time.Until(p.expiresAt) > time.Minute {
		token := p.cachedToken
		p.mu.Unlock()
		return token, nil
	}
	p.mu.Unlock()

	assertion, err := p.buildJWTAssertion()
	if err != nil {
		return "", err
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build google token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("google token request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", fmt.Errorf("read google token response: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("google token request failed: status=%d body=%s", resp.StatusCode, respBody)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return "", fmt.Errorf("decode google token response: %w", err)
	}
	if strings.TrimSpace(tokenResp.AccessToken) == "" {
		return "", fmt.Errorf("google token response has empty access_token")
	}
	if tokenResp.ExpiresIn <= 0 {
		tokenResp.ExpiresIn = 3600
	}

	p.mu.Lock()
	p.cachedToken = tokenResp.AccessToken
	p.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	p.mu.Unlock()

	return tokenResp.AccessToken, nil
}

func (p *serviceAccountTokenProvider) buildJWTAssertion() (string, error) {
	privateKey, err := parseRSAPrivateKey(p.privateKey)
	if err != nil {
		return "", err
	}

	headerJSON, err := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	})
	if err != nil {
		return "", fmt.Errorf("marshal jwt header: %w", err)
	}

	now := time.Now().Unix()
	claimsJSON, err := json.Marshal(map[string]any{
		"iss":   p.email,
		"scope": "https://www.googleapis.com/auth/calendar",
		"aud":   p.tokenURI,
		"iat":   now,
		"exp":   now + 3600,
	})
	if err != nil {
		return "", fmt.Errorf("marshal jwt claims: %w", err)
	}

	unsigned := encodeBase64URL(headerJSON) + "." + encodeBase64URL(claimsJSON)
	hash := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("sign jwt assertion: %w", err)
	}
	return unsigned + "." + encodeBase64URL(signature), nil
}

func parseRSAPrivateKey(value string) (*rsa.PrivateKey, error) {
	normalized := strings.ReplaceAll(value, `\n`, "\n")
	block, _ := pem.Decode([]byte(normalized))
	if block == nil {
		return nil, fmt.Errorf("failed to decode service account private key PEM")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse service account private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("service account private key is not RSA")
	}
	return key, nil
}

func encodeBase64URL(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}
