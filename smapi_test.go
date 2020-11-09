package smapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
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

	instancesByOrg = map[int64][]HostedInstance{
		1000: {
			{
				ID:   1,
				Type: InstanceTypePrometheus,
				Name: "org-1000-prom",
				URL:  "https://prometheus.grafana",
			},
			{
				ID:   2,
				Type: InstanceTypeLogs,
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

func (r *InitRequest) GetAdminToken() string {
	return r.AdminToken
}

func (r *SaveRequest) GetAdminToken() string {
	return r.AdminToken
}

func TestClientInit(t *testing.T) {
	url, mux, cleanup := newTestServer(t)
	defer cleanup()

	mux.Handle("/api/v1/register/init", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req InitRequest

		orgId := readPostRequest(w, r, &req)
		if orgId < 0 {
			return
		}

		tenantId, ok := tenantsByOrg[orgId]
		if !ok {
			errorResponse(w, http.StatusInternalServerError, "cannot get tenant")
			return
		}

		resp := InitResponse{
			AccessToken: tokensByTenant[tenantId],
			TenantInfo: &TenantDescription{
				ID:             tenantId,
				MetricInstance: HostedInstance{},
				LogInstance:    HostedInstance{},
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
	require.Equal(t, tokensByTenant[2000], resp.AccessToken)
	require.NotNil(t, resp.TenantInfo)
	require.Equal(t, tenantsByOrg[1000], resp.TenantInfo.ID)
	require.NotNil(t, resp.Instances)
	require.ElementsMatch(t, resp.Instances, instancesByOrg[1000])
	require.Equal(t, resp.AccessToken, c.accessToken, "client access token should be set after successful init call")
}

func TestClientSave(t *testing.T) {
	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/register/save", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req SaveRequest

		orgId := readPostRequest(w, r, &req)
		if orgId < 0 {
			return
		}

		_, ok := tenantsByOrg[orgId]
		if !ok {
			errorResponse(w, http.StatusInternalServerError, "cannot get tenant")
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

		resp := SaveResponse{}

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
	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/probe/add", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req synthetic_monitoring.Probe
		tenantId := readPostRequest(w, r, &req)
		if tenantId < 0 {
			return
		}

		probeIds, found := probesByTenantId[tenantId]
		if !found {
			errorResponse(w, http.StatusInternalServerError, "no probes for this tenant")
			return
		}

		resp := ProbeAddResponse{
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

	tenantId := int64(2000)
	c := NewClient(url, tokensByTenant[tenantId], http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	probe := synthetic_monitoring.Probe{}
	newProbe, probeToken, err := c.AddProbe(ctx, probe)

	require.NoError(t, err)
	require.NotNil(t, newProbe)
	require.NotZero(t, newProbe.Id)
	require.Equal(t, tenantId, newProbe.TenantId)
	require.Greater(t, newProbe.OnlineChange, float64(0))
	require.Greater(t, newProbe.Created, float64(0))
	require.Greater(t, newProbe.Modified, float64(0))
	require.Empty(t, cmp.Diff(&probe, newProbe, ignoreIdField, ignoreTenantIdField, ignoreTimeFields),
		"AddProbe mismatch (-want +got)")
	require.Equal(t, probeTokensById[newProbe.Id], probeToken)
}

func TestDeleteProbe(t *testing.T) {
	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/probe/delete/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			errorResponse(w, http.StatusBadRequest, "invalid request method")
			return
		}

		tenantId := authorizeTenant(w, r)
		if tenantId < 0 {
			// the tenant exists, but the authorization is incorrect
			return
		} else if tenantId == 0 {
			// the tenant does not exist
			errorResponse(w, http.StatusUnauthorized, "invalid authorization credentials")
			return
		}

		probeId, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/v1/probe/delete/"), 10, 64)
		if err != nil {
			errorResponse(w, http.StatusBadRequest, "invalid probe ID")
			return
		}

		found := false

		for _, id := range probesByTenantId[tenantId] {
			if id == probeId {
				found = true
				break
			}
		}

		if !found {
			errorResponse(w, http.StatusBadRequest, "invalid probe ID")
			return
		}

		resp := ProbeDeleteResponse{
			Msg:     "probe deleted",
			ProbeID: probeId,
		}

		writeResponse(w, http.StatusOK, &resp)
	}))

	tenantId := int64(2000)
	c := NewClient(url, tokensByTenant[tenantId], http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.DeleteProbe(ctx, probesByTenantId[tenantId][0])
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

func readPostRequest(w http.ResponseWriter, r *http.Request, req interface{}) int64 {
	if r.Body == nil {
		errorResponse(w, http.StatusBadRequest, "invalid request")
		return -1
	}
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	err := dec.Decode(req)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "cannot decode request")
		return -2
	}

	if req, ok := req.(AdminTokenGetter); ok {
		orgId, ok := orgsByToken[req.GetAdminToken()]
		if !ok {
			errorResponse(w, http.StatusUnauthorized, "not authorized")
			return -3
		}
		return orgId
	}

	if tenantId := authorizeTenant(w, r); tenantId != 0 {
		return tenantId
	}

	errorResponse(w, http.StatusUnauthorized, "invalid authorization credentials")
	return -4
}

func authorizeTenant(w http.ResponseWriter, r *http.Request) int64 {
	if authHeader := r.Header.Get("authorization"); authHeader != "" {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		tenantId, ok := tenantsByToken[token]
		if !ok {
			errorResponse(w, http.StatusUnauthorized, "not authorized")
			return -10
		}
		return tenantId
	}

	return 0 // no action here
}

func writeResponse(w http.ResponseWriter, code int, resp interface{}) {
	enc := json.NewEncoder(w)
	w.WriteHeader(code)
	_ = enc.Encode(resp)
}

func errorResponse(w http.ResponseWriter, code int, msg string) {
	writeResponse(w, http.StatusInternalServerError, &ErrorResponse{Msg: msg})
}
