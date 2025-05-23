// Package smapi provides access to the Synthetic Monitoring API.
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

var (
	// ErrAuthorizationTokenRequired is the error returned by client
	// calls that require an authorization token.
	//
	// Authorization tokens can be obtained using Install or Init.
	ErrAuthorizationTokenRequired = errors.New("authorization token required")

	// ErrCannotEncodeJSONRequest is the error returned if it's not
	// possible to encode the request as a JSON object. This error
	// should never happen.
	ErrCannotEncodeJSONRequest = errors.New("cannot encode request")

	// ErrUnexpectedResponse is returned by client calls that get
	// unexpected responses, for example some field that must not be
	// zero is zero. If possible a more specific error should be
	// used.
	ErrUnexpectedResponse = errors.New("unexpected response")
)

// Client is a Synthetic Monitoring API client.
//
// It should be initialized using the NewClient function in this package.
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

// NewDatasourceClient creates a new client for the Synthetic Monitoring API using a Grafana datasource proxy.
//
// The accessToken should be the grafana access token.
//
// If no client is provided, http.DefaultClient will be used.
func NewDatasourceClient(baseURL, accessToken string, client *http.Client) *Client {
	if client == nil {
		client = http.DefaultClient
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}
	u.Path = strings.TrimSuffix(u.Path, "/")

	return &Client{
		client:      client,
		accessToken: accessToken,
		baseURL:     u.String(),
	}
}

// Install takes a stack ID, a hosted metrics instance ID, a hosted logs
// instance ID and a publisher token that can be used to publish data to those
// instances and sets up a new Synthetic Monitoring tenant using those
// parameters.
//
// Note that the client will not any validation on these arguments and it will
// simply pass them to the corresponding API server.
//
// The returned RegistrationInstallResponse will contain the access token used
// to make further calls to the API server. This call will _modify_ the client
// in order to use that access token.
func (h *Client) Install(ctx context.Context, stackID, metricsInstanceID, logsInstanceID int64, publisherToken string) (*model.RegistrationInstallResponse, error) {
	request := model.RegistrationInstallRequest{
		LogsInstanceID:    logsInstanceID,
		MetricsInstanceID: metricsInstanceID,
		StackID:           stackID,
	}

	buf, err := json.Marshal(&request)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling install request: %w", err)
	}

	body := bytes.NewReader(buf)

	headers := defaultHeaders()
	headers.Set("Authorization", "Bearer "+publisherToken)

	resp, err := h.Post(ctx, "/register/install", false, headers, body)
	if err != nil {
		return nil, fmt.Errorf("sending install request: %w", err)
	}

	var result model.RegistrationInstallResponse

	if err := ValidateResponse("registration install request", resp, &result); err != nil {
		return nil, err
	}

	h.accessToken = result.AccessToken

	return &result, nil
}

// CreateToken is used to obtain a new access token for the
// authenticated tenant.
//
// The newly created token _does not_ replace the token currently
// used by the client.
func (h *Client) CreateToken(ctx context.Context) (string, error) {
	if err := h.requireAuthToken(); err != nil {
		return "", err
	}

	resp, err := h.PostJSON(ctx, "/token/create", true, nil)
	if err != nil {
		return "", fmt.Errorf("creating token: %w", err)
	}

	var result model.TokenCreateResponse

	if err := ValidateResponse("token create request", resp, &result); err != nil {
		return "", err
	}

	return result.AccessToken, nil
}

// DeleteToken deletes the access token that the client is currently
// using.
//
// After this call, the client won't be able to make furhter calls into
// the API.
func (h *Client) DeleteToken(ctx context.Context) error {
	if err := h.requireAuthToken(); err != nil {
		return err
	}

	resp, err := h.Delete(ctx, fmt.Sprintf("%s%s", h.baseURL, "/token/delete"), true)
	if err != nil {
		return fmt.Errorf("deleting token: %w", err)
	}

	var result model.TokenDeleteResponse

	if err := ValidateResponse("token delete request", resp, &result); err != nil {
		return err
	}

	// the token is no longer valid, remove it from the client so
	// that future calls fail.
	h.accessToken = ""

	return nil
}

// RefreshToken creates a new access token in the API which replaces the
// one that the client is currently using.
func (h *Client) RefreshToken(ctx context.Context) error {
	if err := h.requireAuthToken(); err != nil {
		return err
	}

	resp, err := h.PostJSON(ctx, "/token/refresh", true, nil)
	if err != nil {
		return fmt.Errorf("refreshing token: %w", err)
	}

	var result model.TokenRefreshResponse

	if err := ValidateResponse("token refresh request", resp, &result); err != nil {
		return err
	}

	if result.AccessToken == "" {
		return ErrUnexpectedResponse
	}

	// replace the existing (now invalid) token with the new one
	h.accessToken = result.AccessToken

	return nil
}

// ValidateToken contacts the API server and verifies that the currently
// installed token is still valid.
// one that the client is currently using.
func (h *Client) ValidateToken(ctx context.Context) error {
	if err := h.requireAuthToken(); err != nil {
		return err
	}

	resp, err := h.PostJSON(ctx, "/token/validate", true, nil)
	if err != nil {
		return fmt.Errorf("validating token: %w", err)
	}

	var result model.TokenValidateResponse

	if err := ValidateResponse("token validate request", resp, &result); err != nil {
		return err
	}

	if !result.IsValid {
		return ErrUnexpectedResponse
	}

	return nil
}

// AddProbe is used to create a new Synthetic Monitoring probe.
//
// The return value includes the assigned probe ID as well as the access token
// that should be used by that probe to communicate with the Synthetic
// Monitoring API.
func (h *Client) AddProbe(ctx context.Context, probe synthetic_monitoring.Probe) (*synthetic_monitoring.Probe, []byte, error) {
	if err := h.requireAuthToken(); err != nil {
		return nil, nil, err
	}

	resp, err := h.PostJSON(ctx, "/probe/add", true, &probe)
	if err != nil {
		return nil, nil, fmt.Errorf("adding probe: %w", err)
	}

	var result model.ProbeAddResponse

	if err := ValidateResponse("probe add request", resp, &result); err != nil {
		return nil, nil, err
	}

	return &result.Probe, result.Token, nil
}

// DeleteProbe is used to remove a new Synthetic Monitoring probe.
func (h *Client) DeleteProbe(ctx context.Context, id int64) error {
	if err := h.requireAuthToken(); err != nil {
		return err
	}

	resp, err := h.Delete(ctx, fmt.Sprintf("%s%s/%d", h.baseURL, "/probe/delete", id), true)
	if err != nil {
		return fmt.Errorf("sending probe delete request: %w", err)
	}

	var result model.ProbeDeleteResponse

	if err := ValidateResponse("probe delete request", resp, &result); err != nil {
		return err
	}

	return nil
}

// UpdateProbe is used to update details about an existing Synthetic Monitoring
// probe.
//
// The return value contains the new representation of the probe according the
// Synthetic Monitoring API server.
func (h *Client) UpdateProbe(ctx context.Context, probe synthetic_monitoring.Probe) (*synthetic_monitoring.Probe, error) {
	if err := h.requireAuthToken(); err != nil {
		return nil, err
	}

	resp, err := h.PostJSON(ctx, "/probe/update", true, &probe)
	if err != nil {
		return nil, fmt.Errorf("sending probe update request: %w", err)
	}

	var result model.ProbeUpdateResponse

	if err := ValidateResponse("probe update request", resp, &result); err != nil {
		return nil, err
	}

	return &result.Probe, nil
}

// ResetProbeToken requests a _new_ token for the probe.
func (h *Client) ResetProbeToken(ctx context.Context, probe synthetic_monitoring.Probe) (*synthetic_monitoring.Probe, []byte, error) {
	if err := h.requireAuthToken(); err != nil {
		return nil, nil, err
	}

	resp, err := h.PostJSON(ctx, "/probe/update?reset-token", true, &probe)
	if err != nil {
		return nil, nil, fmt.Errorf("sending probe update request: %w", err)
	}

	var result model.ProbeUpdateResponse

	if err := ValidateResponse("probe update request", resp, &result); err != nil {
		return nil, nil, err
	}

	return &result.Probe, result.Token, nil
}

// GetProbe is used to obtain the details about a single existing
// Synthetic Monitoring probe.
func (h *Client) GetProbe(ctx context.Context, id int64) (*synthetic_monitoring.Probe, error) {
	if err := h.requireAuthToken(); err != nil {
		return nil, err
	}

	resp, err := h.Get(ctx, fmt.Sprintf("/probe/%d", id), true, nil)
	if err != nil {
		return nil, fmt.Errorf("sending probe get request: %w", err)
	}

	var result synthetic_monitoring.Probe

	if err := ValidateResponse("probe get request", resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ListProbes returns the list of probes accessible to the authenticated
// tenant.
func (h *Client) ListProbes(ctx context.Context) ([]synthetic_monitoring.Probe, error) {
	if err := h.requireAuthToken(); err != nil {
		return nil, err
	}

	resp, err := h.Get(ctx, "/probe/list", true, nil)
	if err != nil {
		return nil, fmt.Errorf("sending probe list request: %w", err)
	}

	var result []synthetic_monitoring.Probe

	if err := ValidateResponse("probe list request", resp, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// AddCheck creates a new Synthetic Monitoring check in the API server.
//
// The return value contains the assigned ID.
func (h *Client) AddCheck(ctx context.Context, check synthetic_monitoring.Check) (*synthetic_monitoring.Check, error) {
	if err := h.requireAuthToken(); err != nil {
		return nil, err
	}

	resp, err := h.PostJSON(ctx, "/check/add", true, &check)
	if err != nil {
		return nil, fmt.Errorf("sending check add request: %w", err)
	}

	var result synthetic_monitoring.Check

	if err := ValidateResponse("check add request", resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetCheck returns a single Synthetic Monitoring check identified by
// the provided ID.
func (h *Client) GetCheck(ctx context.Context, id int64) (*synthetic_monitoring.Check, error) {
	if err := h.requireAuthToken(); err != nil {
		return nil, err
	}

	resp, err := h.Get(ctx, fmt.Sprintf("/check/%d", id), true, nil)
	if err != nil {
		return nil, fmt.Errorf("sending check get request: %w", err)
	}

	var result synthetic_monitoring.Check

	if err := ValidateResponse("check get request", resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// UpdateCheck updates an existing check in the API server.
//
// The return value contains the updated check (updated timestamps,
// etc).
func (h *Client) UpdateCheck(ctx context.Context, check synthetic_monitoring.Check) (*synthetic_monitoring.Check, error) {
	if err := h.requireAuthToken(); err != nil {
		return nil, err
	}

	resp, err := h.PostJSON(ctx, "/check/update", true, &check)
	if err != nil {
		return nil, fmt.Errorf("sending check update request: %w", err)
	}

	var result synthetic_monitoring.Check

	if err := ValidateResponse("check update request", resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// DeleteCheck deletes an existing Synthetic Monitoring check from the API
// server.
func (h *Client) DeleteCheck(ctx context.Context, id int64) error {
	if err := h.requireAuthToken(); err != nil {
		return err
	}

	resp, err := h.Delete(ctx, fmt.Sprintf("%s%s/%d", h.baseURL, "/check/delete", id), true)
	if err != nil {
		return fmt.Errorf("sending check delete request: %w", err)
	}

	var result model.CheckDeleteResponse

	if err := ValidateResponse("check delete request", resp, &result); err != nil {
		return err
	}

	return nil
}

// ListChecks returns the list of Synthetic Monitoring checks for the
// authenticated tenant.
func (h *Client) ListChecks(ctx context.Context) ([]synthetic_monitoring.Check, error) {
	if err := h.requireAuthToken(); err != nil {
		return nil, err
	}

	resp, err := h.Get(ctx, "/check/list", true, nil)
	if err != nil {
		return nil, fmt.Errorf("sending check list request: %w", err)
	}

	var result []synthetic_monitoring.Check

	if err := ValidateResponse("check list request", resp, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetTenant retrieves the information associated with the authenticated
// tenant.
func (h *Client) GetTenant(ctx context.Context) (*synthetic_monitoring.Tenant, error) {
	if err := h.requireAuthToken(); err != nil {
		return nil, err
	}

	resp, err := h.Get(ctx, "/tenant", true, nil)
	if err != nil {
		return nil, fmt.Errorf("sending get tenant request: %w", err)
	}

	var result synthetic_monitoring.Tenant

	if err := ValidateResponse("get tenant request", resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// UpdateTenant updates the specified tenant in the Synthetic Monitoring
// API. The updated tenant (possibly with updated timestamps) is
// returned.
func (h *Client) UpdateTenant(ctx context.Context, tenant synthetic_monitoring.Tenant) (*synthetic_monitoring.Tenant, error) {
	if err := h.requireAuthToken(); err != nil {
		return nil, err
	}

	resp, err := h.PostJSON(ctx, "/tenant/update", true, &tenant)
	if err != nil {
		return nil, fmt.Errorf("sending tenant update request: %w", err)
	}

	var result synthetic_monitoring.Tenant

	if err := ValidateResponse("tenant update request", resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
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
		return nil, fmt.Errorf("creating new HTTP request: %w", err)
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

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending HTTP request: %w", err)
	}

	return resp, nil
}

// Get is a utility method to send a GET request to the SM API.
//
// The `url` argument specifies the additional URL path of the request (minus
// the base, which is part of the client). `auth` specifies whether or not to
// include authorization headers. `headers` specifies any additional headers
// that need to be included with the request.
func (h *Client) Get(ctx context.Context, url string, auth bool, headers http.Header) (*http.Response, error) {
	return h.do(ctx, h.baseURL+url, http.MethodGet, auth, headers, nil)
}

// Post is a utility method to send a POST request to the SM API.
//
// The `url` argument specifies the additional URL path of the request (minus
// the base, which is part of the client). `auth` specifies whether or not to
// include authorization headers. `headers` specifies any additional headers
// that need to be included with the request. `body` is the body sent with the
// POST request.
func (h *Client) Post(ctx context.Context, url string, auth bool, headers http.Header, body io.Reader) (*http.Response, error) {
	return h.do(ctx, h.baseURL+url, http.MethodPost, auth, headers, body)
}

// PostJSON is a utility method to send a POST request to the SM API with a
// body specified by the `req` argument encoded as JSON.
//
// The `url` argument specifies the additional URL path of the request (minus
// the base, which is part of the client). `auth` specifies whether or not to
// include authorization headers. `headers` specifies any additional headers
// that need to be included with the request.
func (h *Client) PostJSON(ctx context.Context, url string, auth bool, req interface{}) (*http.Response, error) {
	var body bytes.Buffer

	var headers http.Header
	if req != nil {
		headers = defaultHeaders()

		if err := json.NewEncoder(&body).Encode(&req); err != nil {
			return nil, ErrCannotEncodeJSONRequest
		}
	}

	return h.Post(ctx, url, auth, headers, &body)
}

// Delete is a utility method to send a DELETE request to the SM API.
//
// The `url` argument specifies the additional URL path of the request (minus
// the base, which is part of the client). `auth` specifies whether or not to
// include authorization headers. `headers` specifies any additional headers
// that need to be included with the request.
func (h *Client) Delete(ctx context.Context, url string, auth bool) (*http.Response, error) {
	return h.do(ctx, url, http.MethodDelete, auth, nil, nil)
}

// Put is a utility method to send a PUT request to the SM API.
//
// The `url` argument specifies the additional URL path of the request (minus
// the base, which is part of the client). `auth` specifies whether or not to
// include authorization headers. `body` specifies the request body, and
// `headers` specifies the request headers.
func (h *Client) Put(ctx context.Context, url string, auth bool, headers http.Header, body io.Reader) (*http.Response, error) {
	return h.do(ctx, h.baseURL+url, http.MethodPut, auth, headers, body)
}

// PutJSON is a utility method to send a PUT request to the SM API with a
// body specified by the `req` argument encoded as JSON.
//
// The `url` argument specifies the additional URL path of the request (minus
// the base, which is part of the client). `auth` specifies whether or not to
// include authorization headers.
func (h *Client) PutJSON(ctx context.Context, url string, auth bool, req interface{}) (*http.Response, error) {
	var body bytes.Buffer

	var headers http.Header
	if req != nil {
		headers = defaultHeaders()

		if err := json.NewEncoder(&body).Encode(&req); err != nil {
			return nil, ErrCannotEncodeJSONRequest
		}
	}

	return h.Put(ctx, url, auth, headers, &body)
}

func (h *Client) UpdateCheckAlerts(ctx context.Context, checkID int64, alerts []model.CheckAlert) ([]model.CheckAlert, error) {
	if err := h.requireAuthToken(); err != nil {
		return nil, err
	}

	request := struct {
		Alerts []model.CheckAlert `json:"alerts"`
	}{
		Alerts: alerts,
	}

	resp, err := h.PutJSON(ctx, fmt.Sprintf("/check/%d/alerts", checkID), true, &request)
	if err != nil {
		return nil, fmt.Errorf("sending check alerts update request: %w", err)
	}

	var result struct {
		Alerts []model.CheckAlert `json:"alerts"`
	}
	if err := ValidateResponse("check alerts update request", resp, &result); err != nil {
		return nil, err
	}

	return result.Alerts, nil
}

func (h *Client) GetCheckAlerts(ctx context.Context, checkID int64) ([]model.CheckAlertWithStatus, error) {
	if err := h.requireAuthToken(); err != nil {
		return nil, err
	}

	resp, err := h.Get(ctx, fmt.Sprintf("/check/%d/alerts", checkID), true, nil)
	if err != nil {
		return nil, fmt.Errorf("sending check alerts get request: %w", err)
	}

	var result struct {
		Alerts []model.CheckAlertWithStatus `json:"alerts"`
	}
	if err := ValidateResponse("check alerts get request", resp, &result); err != nil {
		return nil, err
	}

	return result.Alerts, nil
}

// HTTPError represents errors returned from the Synthetic Monitoring API
// server.
//
// It implements the error interface, so it can be returned from functions
// interacting with the Synthetic Monitoring API server.
type HTTPError struct {
	Code   int
	Status string
	Action string
	Api    struct {
		Msg   string
		Error string
	}
}

// Error allows HTTPError to implement the error interface.
//
// The formatting of the error is a little opinionated, as it has to
// communicate an error from the API if it's there, or an error from the
// HTTP client.
func (e *HTTPError) Error() string {
	if e.Api.Msg != "" || e.Api.Error != "" {
		return fmt.Sprintf("%s: status=\"%s\", msg=\"%s\", err=\"%s\"", e.Action, e.Status, e.Api.Msg, e.Api.Error)
	}

	return fmt.Sprintf("%s: status=\"%s\"", e.Action, e.Status)
}

func defaultHeaders() http.Header {
	headers := make(http.Header)
	headers.Set("Content-type", "application/json; charset=utf-8")

	return headers
}

// ValidateResponse handles responses from the SM API.
//
// If the status code of the request is not 200 or 202, it is expected that there's an
// error included with the response. This function will decode that response
// and return in the form of an HTTPError.
//
// In the case of success, this function attempts to decode the response as a
// JSON object and storing it the `result` argument.
func ValidateResponse(action string, resp *http.Response, result interface{}) error {
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		respError := HTTPError{Code: resp.StatusCode, Status: resp.Status, Action: action}

		if resp.Body != nil {
			defer resp.Body.Close()

			dec := json.NewDecoder(resp.Body)

			var apiError struct {
				Error string `json:"err"`
				Msg   string `json:"msg"`
			}

			if err := dec.Decode(&apiError); err != nil {
				// If there's an error decoding this,
				// it's not something we can deal with,
				// so don't add additional annotations.
				respError.Api.Msg = "cannot decode response"
				respError.Api.Error = err.Error()
			} else {
				respError.Api.Msg = apiError.Msg
				respError.Api.Error = apiError.Error
			}
		}

		return &respError
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
