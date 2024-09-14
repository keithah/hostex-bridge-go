package hostexapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	logger     *zap.Logger
}

type Conversation struct {
	ID            string    `json:"id"`
	ChannelType   string    `json:"channel_type"`
	LastMessageAt time.Time `json:"last_message_at"`
	Guest         struct {
		Name  string `json:"name"`
		Phone string `json:"phone"`
		Email string `json:"email"`
	} `json:"guest"`
	PropertyTitle string `json:"property_title"`
	CheckInDate   string `json:"check_in_date"`
	CheckOutDate  string `json:"check_out_date"`
}

type Message struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Sender    string    `json:"sender"`
}

type ConversationsResponse struct {
	RequestID string `json:"request_id"`
	ErrorCode int    `json:"error_code"`
	ErrorMsg  string `json:"error_msg"`
	Data      struct {
		Conversations []Conversation `json:"conversations"`
	} `json:"data"`
}

type MessagesResponse struct {
	RequestID string `json:"request_id"`
	ErrorCode int    `json:"error_code"`
	ErrorMsg  string `json:"error_msg"`
	Data      struct {
		Messages []Message `json:"messages"`
	} `json:"data"`
}

func NewClient(baseURL, token string, logger *zap.Logger) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

func (c *Client) GetConversations() ([]Conversation, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/conversations", c.baseURL), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Hostex-Access-Token", c.token)
	req.Header.Set("User-Agent", "HostexBridge/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status code: %d", resp.StatusCode)
	}

	var conversationsResp ConversationsResponse
	err = json.NewDecoder(resp.Body).Decode(&conversationsResp)
	if err != nil {
		return nil, err
	}

	if conversationsResp.ErrorCode != 200 {
		return nil, fmt.Errorf("API error: %s", conversationsResp.ErrorMsg)
	}

	return conversationsResp.Data.Conversations, nil
}

func (c *Client) GetMessages(conversationID string, since time.Time, limit int) ([]Message, error) {
	url := fmt.Sprintf("%s/conversations/%s/messages?since=%s&limit=%d", c.baseURL, conversationID, since.Format(time.RFC3339), limit)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Hostex-Access-Token", c.token)
	req.Header.Set("User-Agent", "HostexBridge/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status code: %d", resp.StatusCode)
	}

	var messagesResp MessagesResponse
	err = json.NewDecoder(resp.Body).Decode(&messagesResp)
	if err != nil {
		return nil, err
	}

	if messagesResp.ErrorCode != 200 {
		return nil, fmt.Errorf("API error: %s", messagesResp.ErrorMsg)
	}

	return messagesResp.Data.Messages, nil
}

func (c *Client) SendMessage(conversationID, content string) error {
	url := fmt.Sprintf("%s/conversations/%s/messages", c.baseURL, conversationID)
	payload := map[string]string{"message": content}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return err
	}

	req.Header.Set("Hostex-Access-Token", c.token)
	req.Header.Set("User-Agent", "HostexBridge/1.0")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API request failed with status code: %d", resp.StatusCode)
	}

	var response struct {
		RequestID string `json:"request_id"`
		ErrorCode int    `json:"error_code"`
		ErrorMsg  string `json:"error_msg"`
	}
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return err
	}

	if response.ErrorCode != 200 {
		return fmt.Errorf("API error: %s", response.ErrorMsg)
	}

	return nil
}
