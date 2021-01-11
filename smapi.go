package smapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/grafana/synthetic-monitoring-api-go-client/model"

	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

// ErrAuthorizationTokenRequired is the error returned by client calls
// that require an authorization token.
//
// Authorization tokens can be obtained using Install or Init.
var ErrAuthorizationTokenRequired = errors.New("authorization token required")

type Client struct {
	client      *http.Client
	accessToken string
	baseURL     string
}

// NewClient creates a new client for the Synthetic Monitoring API.
//
// The accessToken is optional. If it's not specified, it's necessary to
// use one of the registration calls to obtain one, Install or Init.
//
// If no client is provided, http.DefaultClient will be used.
func NewClient(baseURL, accessToken string, client *http.Client) *Client {
	if client == nil {
		client = http.DefaultClient
	}

	u, err := url.Parse(baseURL + "/api/v1")
	if err != nil {
		return nil
	}

	u.Path = path.Clean(u.Path)

	return &Client{
		client:      client,
		accessToken: accessToken,
		baseURL:     u.String(),
	}
}

func (h *Client) Install(ctx context.Context, stackID, metricsInstanceID, logsInstanceID int64, publisherToken string) (*model.RegistrationInstallResponse, error) {
	request := model.RegistrationInstallRequest{
		LogsInstanceID:    logsInstanceID,
		MetricsInstanceID: metricsInstanceID,
		StackID:           stackID,
	}

	buf, err := json.Marshal(&request)
	if err != nil {
		return nil, err
	}

	body := bytes.NewReader(buf)

	headers := defaultHeaders()
	headers.Set("Authorization", "Bearer "+publisherToken)

	resp, err := h.post(ctx, h.baseURL+"/register/install", false, headers, body)
	if err != nil {
		return nil, fmt.Errorf("sending install request: %w", err)
	}

	var result model.RegistrationInstallResponse

	if err := validateResponse("registration install request", resp, &result); err != nil {
		return nil, err
	}

	h.accessToken = result.AccessToken

	return &result, nil
}

func (h *Client) Init(ctx context.Context, adminToken string) (*model.InitResponse, error) {
	body := strings.NewReader(`{"apiToken": "` + adminToken + `"}`)

	resp, err := h.postJSON(ctx, h.baseURL+"/register/init", false, body)
	if err != nil {
		return nil, fmt.Errorf("sending init request: %w", err)
	}

	var result model.InitResponse

	if err := validateResponse("registration init request", resp, &result); err != nil {
		return nil, err
	}

	h.accessToken = result.AccessToken

	return &result, nil
}

func (h *Client) Save(ctx context.Context, adminToken string, metricInstanceID, logInstanceID int64) error {
	saveReq := struct {
		AdminToken        string `json:"apiToken"`
		MetricsInstanceID int64  `json:"metricsInstanceId"`
		LogsInstanceID    int64  `json:"logsInstanceId"`
	}{
		AdminToken:        adminToken,
		MetricsInstanceID: metricInstanceID,
		LogsInstanceID:    logInstanceID,
	}

	var body bytes.Buffer

	enc := json.NewEncoder(&body)

	if err := enc.Encode(&saveReq); err != nil {
		return fmt.Errorf("cannot encode request")
	}

	resp, err := h.postJSON(ctx, h.baseURL+"/register/save", true, &body)
	if err != nil {
		return fmt.Errorf("sending save request: %w", err)
	}

	var result struct{}

	if err := validateResponse("registration save request", resp, &result); err != nil {
		return err
	}

	return nil
}

func (h *Client) AddProbe(ctx context.Context, probe synthetic_monitoring.Probe) (*synthetic_monitoring.Probe, []byte, error) {
	if err := h.requireAuthToken(); err != nil {
		return nil, nil, err
	}

	body, err := json.Marshal(&probe)
	if err != nil {
		return nil, nil, err
	}

	resp, err := h.postJSON(ctx, h.baseURL+"/probe/add", true, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("adding probe: %w", err)
	}

	var result model.ProbeAddResponse

	if err := validateResponse("probe add request", resp, &result); err != nil {
		return nil, nil, err
	}

	return &result.Probe, result.Token, nil
}

func (h *Client) DeleteProbe(ctx context.Context, id int64) error {
	if err := h.requireAuthToken(); err != nil {
		return err
	}

	resp, err := h.delete(ctx, fmt.Sprintf("%s%s/%d", h.baseURL, "/probe/delete", id), true)
	if err != nil {
		return fmt.Errorf("sending probe delete request: %w", err)
	}

	var result model.ProbeDeleteResponse

	if err := validateResponse("probe delete request", resp, &result); err != nil {
		return err
	}

	return nil
}

func (h *Client) UpdateProbe(ctx context.Context, probe synthetic_monitoring.Probe) (*synthetic_monitoring.Probe, error) {
	if err := h.requireAuthToken(); err != nil {
		return nil, err
	}

	body, err := json.Marshal(&probe)
	if err != nil {
		return nil, err
	}

	resp, err := h.postJSON(ctx, h.baseURL+"/probe/update", true, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("sending probe update request: %w", err)
	}

	var result model.ProbeUpdateResponse

	if err := validateResponse("probe update request", resp, &result); err != nil {
		return nil, err
	}

	return &result.Probe, nil
}

func (h *Client) ResetProbeToken(ctx context.Context, probe synthetic_monitoring.Probe) (*synthetic_monitoring.Probe, []byte, error) {
	if err := h.requireAuthToken(); err != nil {
		return nil, nil, err
	}

	body, err := json.Marshal(&probe)
	if err != nil {
		return nil, nil, err
	}

	resp, err := h.postJSON(ctx, h.baseURL+"/probe/update?reset-token", true, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("sending probe update request: %w", err)
	}

	var result model.ProbeUpdateResponse

	if err := validateResponse("probe update request", resp, &result); err != nil {
		return nil, nil, err
	}

	return &result.Probe, result.Token, nil
}

func (h *Client) AddCheck(ctx context.Context, check synthetic_monitoring.Check) (*synthetic_monitoring.Check, error) {
	if err := h.requireAuthToken(); err != nil {
		return nil, err
	}

	body, err := json.Marshal(&check)
	if err != nil {
		return nil, err
	}

	resp, err := h.postJSON(ctx, h.baseURL+"/check/add", true, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("sending check add request: %w", err)
	}

	var result synthetic_monitoring.Check

	if err := validateResponse("check add request", resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (h *Client) DeleteCheck(ctx context.Context, id int64) error {
	if err := h.requireAuthToken(); err != nil {
		return err
	}

	resp, err := h.delete(ctx, fmt.Sprintf("%s%s/%d", h.baseURL, "/check/delete", id), true)
	if err != nil {
		return fmt.Errorf("sending check delete request: %w", err)
	}

	var result model.CheckDeleteResponse

	if err := validateResponse("check delete request", resp, &result); err != nil {
		return err
	}

	return nil
}

func (h *Client) requireAuthToken() error {
	if h.accessToken == "" {
		return ErrAuthorizationTokenRequired
	}

	return nil
}

func (h *Client) do(ctx context.Context, url, method string, auth bool, headers http.Header, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	if headers != nil {
		req.Header = headers
	}

	if auth {
		if req.Header == nil {
			req.Header = make(http.Header)
		}
		req.Header.Set("Authorization", "Bearer "+h.accessToken)
	}

	return h.client.Do(req)
}

func (h *Client) post(ctx context.Context, url string, auth bool, headers http.Header, body io.Reader) (*http.Response, error) {
	return h.do(ctx, url, http.MethodPost, auth, headers, body)
}

func (h *Client) postJSON(ctx context.Context, url string, auth bool, body io.Reader) (*http.Response, error) {
	var headers http.Header
	if body != nil {
		headers = defaultHeaders()
	}

	return h.post(ctx, url, auth, headers, body)
}

func (h *Client) delete(ctx context.Context, url string, auth bool) (*http.Response, error) {
	return h.do(ctx, url, http.MethodDelete, auth, nil, nil)
}

type HttpError struct {
	Code   int
	Status string
	Action string
}

func (e *HttpError) Error() string {
	return fmt.Sprintf("%s: %s", e.Action, e.Status)
}

func defaultHeaders() http.Header {
	headers := make(http.Header)
	headers.Set("Content-type", "application/json; charset=utf-8")
	return headers
}

func validateResponse(action string, resp *http.Response, result interface{}) error {
	if resp.StatusCode != http.StatusOK {
		return &HttpError{Code: resp.StatusCode, Status: resp.Status, Action: action}
	}

	if resp.Body != nil {
		defer resp.Body.Close()

		dec := json.NewDecoder(resp.Body)

		if err := dec.Decode(result); err != nil {
			return fmt.Errorf("%s, decoding response: %w", action, err)
		}
	}

	return nil
}
