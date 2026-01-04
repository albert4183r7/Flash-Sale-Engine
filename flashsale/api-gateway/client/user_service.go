package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type UserClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewUserClient(baseURL string) *UserClient {
	return &UserClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type SignupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type SignupResponse struct {
	Message string    `json:"message"`
	UserID  uuid.UUID `json:"user_id,omitempty"`
	Error   string    `json:"error,omitempty"`
}

func (c *UserClient) Signup(email, password string) (*SignupResponse, int, error) {
	req := SignupRequest{Email: email, Password: password}
	body, _ := json.Marshal(req)

	resp, err := c.httpClient.Post(c.baseURL+"/signup", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	var result SignupResponse
	json.NewDecoder(resp.Body).Decode(&result)
	return &result, resp.StatusCode, nil
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginData struct {
	Token     string    `json:"token"`
	ExpiresIn int64     `json:"expires_in"`
	UserID    uuid.UUID `json:"user_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
}

type LoginResponse struct {
	Data  LoginData `json:"data,omitempty"`
	Error string    `json:"error,omitempty"`
}

func (c *UserClient) Login(email, password string) (*LoginResponse, int, error) {
	req := LoginRequest{Email: email, Password: password}
	body, _ := json.Marshal(req)

	resp, err := c.httpClient.Post(c.baseURL+"/login", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	var result LoginResponse
	json.NewDecoder(resp.Body).Decode(&result)
	return &result, resp.StatusCode, nil
}

type ValidateTokenRequest struct {
	Token string `json:"token"`
}

type ValidateTokenResponse struct {
	Valid  bool      `json:"valid"`
	UserID uuid.UUID `json:"user_id,omitempty"`
	Email  string    `json:"email,omitempty"`
	Role   string    `json:"role,omitempty"`
	Error  string    `json:"error,omitempty"`
}

func (c *UserClient) ValidateToken(token string) (*ValidateTokenResponse, error) {
	req := ValidateTokenRequest{Token: token}
	body, _ := json.Marshal(req)

	resp, err := c.httpClient.Post(c.baseURL+"/internal/validate-token", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result ValidateTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

type UserResponse struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Role  string    `json:"role"`
	Error string    `json:"error,omitempty"`
}

func (c *UserClient) GetUser(userID uuid.UUID) (*UserResponse, error) {
	resp, err := c.httpClient.Get(fmt.Sprintf("%s/internal/users/%s", c.baseURL, userID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result UserResponse
	json.NewDecoder(resp.Body).Decode(&result)
	return &result, nil
}
