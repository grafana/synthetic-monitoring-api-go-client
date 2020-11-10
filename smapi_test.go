package smapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/grafana/synthetic-monitoring-api-go-client/model"
	"github.com/stretchr/testify/require"
)

var (
	orgsByToken = map[string]int64{
		"token-org-1000": 1000,
	}

	tenantsByOrg = map[int64]int64{
		1000: 2000,
	}

	tokensByTenant = map[int64]string{
		2000: "token-tenant-2000",
	}

	tenantsByToken = func() map[string]int64 {
		m := make(map[string]int64)
		for k, v := range tokensByTenant {
			m[v] = k
		}
		return m
	}()

	instancesByOrg = map[int64][]model.HostedInstance{
		1000: {
			{
				ID:   1,
				Type: model.InstanceTypePrometheus,
				Name: "org-1000-prom",
				URL:  "https://prometheus.grafana",
			},
			{
				ID:   2,
				Type: model.InstanceTypeLogs,
				Name: "org-1000-logs",
				URL:  "https://logs.grafana",
			},
		},
	}

	probesByTenantId = map[int64][]int64{
		2000: {1},
	}

	probeTokensById = map[int64][]byte{
		1: {0x01, 0x02, 0x03, 0x04},
	}
)

type AdminTokenGetter interface {
	GetAdminToken() string
}

type InitRequest struct {
	model.InitRequest
}

func (r *InitRequest) GetAdminToken() string {
	return r.AdminToken
}

type SaveRequest struct {
	model.SaveRequest
}

func (r *SaveRequest) GetAdminToken() string {
	return r.AdminToken
}

func TestClientInit(t *testing.T) {
	testTenantId := int64(2000)

	url, mux, cleanup := newTestServer(t)
	defer cleanup()

	mux.Handle("/api/v1/register/init", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req InitRequest

		orgId := readPostRequest(w, r, &req, -1000)
		if orgId < 0 {
			return
		}

		tenantId, ok := tenantsByOrg[orgId]
		if !ok {
			errorResponse(w, http.StatusInternalServerError, "cannot get tenant")
			return
		}

		resp := model.InitResponse{
			AccessToken: tokensByTenant[tenantId],
			TenantInfo: &model.TenantDescription{
				ID:             tenantId,
				MetricInstance: model.HostedInstance{},
				LogInstance:    model.HostedInstance{},
			},
			Instances: instancesByOrg[orgId],
		}

		writeResponse(w, http.StatusOK, &resp)
	}))

	adminToken := "token-org-1000"

	c := NewClient(url, "", http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Init(ctx, adminToken)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, tokensByTenant[testTenantId], resp.AccessToken)
	require.NotNil(t, resp.TenantInfo)
	require.Equal(t, tenantsByOrg[1000], resp.TenantInfo.ID)
	require.NotNil(t, resp.Instances)
	require.ElementsMatch(t, resp.Instances, instancesByOrg[1000])
	require.Equal(t, resp.AccessToken, c.accessToken, "client access token should be set after successful init call")
}

func TestClientSave(t *testing.T) {
	testTenantId := int64(2000)

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/register/save", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req SaveRequest

		orgId := readPostRequest(w, r, &req, -1000)
		if orgId < 0 {
			return
		}

		tenantId, ok := tenantsByOrg[orgId]
		if !ok {
			errorResponse(w, http.StatusInternalServerError, "cannot get tenant")
			return
		}
		if tenantId != testTenantId {
			errorResponse(w, http.StatusExpectationFailed, fmt.Sprintf("expecting tenant ID %d, got %d", testTenantId, tenantId))
			return
		}

		if req.MetricInstanceId <= 0 {
			errorResponse(w, http.StatusBadRequest, "invalid metrics instance ID")
			return
		}

		if req.LogInstanceId <= 0 {
			errorResponse(w, http.StatusBadRequest, "invalid logs instance ID")
			return
		}

		resp := model.SaveResponse{}

		writeResponse(w, http.StatusOK, &resp)
	}))

	adminToken := "token-org-1000"
	c := NewClient(url, "", http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.Save(ctx, adminToken, instancesByOrg[1000][0].ID, instancesByOrg[1000][1].ID)
	require.NoError(t, err)
}

func TestAddProbe(t *testing.T) {
	testTenantId := int64(2000)

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/probe/add", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req synthetic_monitoring.Probe
		tenantId := readPostRequest(w, r, &req, testTenantId)
		if tenantId < 0 {
			return
		}
		if tenantId != testTenantId {
			errorResponse(w, http.StatusExpectationFailed, fmt.Sprintf("expecting tenant ID %d, got %d", testTenantId, tenantId))
			return
		}

		probeIds, found := probesByTenantId[tenantId]
		if !found {
			errorResponse(w, http.StatusInternalServerError, "no probes for this tenant")
			return
		}

		resp := model.ProbeAddResponse{
			Token: []byte{0x01, 0x02, 0x03, 0x04},
		}

		resp.Probe = req
		resp.Probe.Id = probeIds[0] // TODO(mem): how to handle multiple probes?
		resp.Probe.TenantId = tenantId
		resp.Probe.OnlineChange = 100
		resp.Probe.Created = 101
		resp.Probe.Modified = 102

		writeResponse(w, http.StatusOK, &resp)
	}))

	c := NewClient(url, tokensByTenant[testTenantId], http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	probe := synthetic_monitoring.Probe{}
	newProbe, probeToken, err := c.AddProbe(ctx, probe)

	require.NoError(t, err)
	require.NotNil(t, newProbe)
	require.NotZero(t, newProbe.Id)
	require.Equal(t, testTenantId, newProbe.TenantId)
	require.Greater(t, newProbe.OnlineChange, float64(0))
	require.Greater(t, newProbe.Created, float64(0))
	require.Greater(t, newProbe.Modified, float64(0))
	require.Empty(t, cmp.Diff(&probe, newProbe, ignoreIdField, ignoreTenantIdField, ignoreTimeFields),
		"AddProbe mismatch (-want +got)")
	require.Equal(t, probeTokensById[newProbe.Id], probeToken)
}

func TestDeleteProbe(t *testing.T) {
	testTenantId := int64(2000)
	testCheckId := int64(42)

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/probe/delete/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := requireMethod(w, r, http.MethodDelete); err != nil {
			return
		}

		if _, err := requireAuth(w, r, testTenantId); err != nil {
			return
		}

		if err := requireId(w, r, testCheckId, "/api/v1/probe/delete/"); err != nil {
			return
		}

		resp := model.ProbeDeleteResponse{
			Msg:     "probe deleted",
			ProbeID: testCheckId,
		}

		writeResponse(w, http.StatusOK, &resp)
	}))

	c := NewClient(url, tokensByTenant[testTenantId], http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.DeleteProbe(ctx, testCheckId)
	require.NoError(t, err)
}

func TestAddCheck(t *testing.T) {
	testTenantId := int64(2000)
	testCheckId := int64(42)

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/check/add", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req synthetic_monitoring.Check
		tenantId := readPostRequest(w, r, &req, testTenantId)
		if tenantId < 0 {
			return
		}

		resp := req

		resp.Id = testCheckId
		resp.TenantId = tenantId
		resp.Created = 200
		resp.Modified = 201

		writeResponse(w, http.StatusOK, &resp)
	}))

	c := NewClient(url, tokensByTenant[testTenantId], http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	check := synthetic_monitoring.Check{}
	newCheck, err := c.AddCheck(ctx, check)

	require.NoError(t, err)
	require.NotNil(t, newCheck)
	require.Equal(t, testCheckId, newCheck.Id)
	require.Equal(t, testTenantId, newCheck.TenantId)
	require.Greater(t, newCheck.Created, float64(0))
	require.Greater(t, newCheck.Modified, float64(0))
	require.Empty(t, cmp.Diff(&check, newCheck, ignoreIdField, ignoreTenantIdField, ignoreTimeFields),
		"AddCheck mismatch (-want +got)")
}

func TestDeleteCheck(t *testing.T) {
	testTenantId := int64(2000)
	testCheckId := int64(42)

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/check/delete/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := requireMethod(w, r, http.MethodDelete); err != nil {
			return
		}

		if _, err := requireAuth(w, r, testTenantId); err != nil {
			return
		}

		if err := requireId(w, r, testCheckId, "/api/v1/check/delete/"); err != nil {
			return
		}

		resp := model.CheckDeleteResponse{
			Msg:     "check deleted",
			CheckID: testCheckId,
		}

		writeResponse(w, http.StatusOK, &resp)
	}))

	c := NewClient(url, tokensByTenant[testTenantId], http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.DeleteCheck(ctx, testCheckId)

	require.NoError(t, err)
}

func newTestServer(t *testing.T) (string, *http.ServeMux, func()) {
	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("req: %s", r.URL.String())
		w.WriteHeader(http.StatusNotImplemented)
	}))

	server := httptest.NewServer(mux)

	return server.URL, mux, server.Close
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) error {
	if r.Method != method {
		errorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid request method %s, expecting %s", r.Method, method))
		return errors.New("bad request")
	}

	return nil
}

func requireId(w http.ResponseWriter, r *http.Request, expected int64, prefix string) error {
	str := strings.TrimPrefix(r.URL.Path, prefix)
	if actual, err := strconv.ParseInt(str, 10, 64); err != nil {
		errorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid ID: %s", str))
		return err
	} else if actual != expected {
		errorResponse(w, http.StatusBadRequest, fmt.Sprintf("expecting ID %d, got %d ", expected, actual))
		return errors.New("unexpected ID")
	}

	return nil
}

func readPostRequest(w http.ResponseWriter, r *http.Request, req interface{}, expectedTenantId int64) int64 {
	if err := requireMethod(w, r, http.MethodPost); err != nil {
		return -1
	}

	if r.Body == nil {
		errorResponse(w, http.StatusBadRequest, "invalid request")
		return -1
	}
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	err := dec.Decode(req)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "cannot decode request")
		return -1
	}

	if req, ok := req.(AdminTokenGetter); ok {
		orgId, ok := orgsByToken[req.GetAdminToken()]
		if !ok {
			errorResponse(w, http.StatusUnauthorized, "not authorized")
			return -1
		}
		return orgId
	}

	if tenantId, err := requireAuth(w, r, expectedTenantId); err == nil {
		return tenantId
	}

	return -1
}

func requireAuth(w http.ResponseWriter, r *http.Request, tenantId int64) (int64, error) {
	authHeader := r.Header.Get("authorization")
	if authHeader == "" {
		return 0, errors.New("no authorization header")
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	actualTenantId, ok := tenantsByToken[token]
	if !ok {
		errorResponse(w, http.StatusUnauthorized, "not authorized")
		return 0, errors.New("not authorized")
	}

	if actualTenantId != tenantId {
		errorResponse(w, http.StatusExpectationFailed, fmt.Sprintf("expecinting tenant ID %d, got %d", tenantId, actualTenantId))
		return 0, errors.New("invalid tenantId")
	}

	return tenantId, nil
}

func writeResponse(w http.ResponseWriter, code int, resp interface{}) {
	enc := json.NewEncoder(w)
	w.WriteHeader(code)
	_ = enc.Encode(resp)
}

func errorResponse(w http.ResponseWriter, code int, msg string) {
	writeResponse(w, code, &model.ErrorResponse{Msg: msg})
}
