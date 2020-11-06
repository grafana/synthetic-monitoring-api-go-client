package smapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

		orgId := readRequest(w, r, &req)
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

		orgId := readRequest(w, r, &req)
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

func newTestServer(t *testing.T) (string, *http.ServeMux, func()) {
	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("req: %s", r.URL.String())
		w.WriteHeader(http.StatusNotImplemented)
	}))

	server := httptest.NewServer(mux)

	return server.URL, mux, server.Close
}

func readRequest(w http.ResponseWriter, r *http.Request, req interface{}) int64 {
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

	errorResponse(w, http.StatusUnauthorized, "invalid authorization credentials")
	return -4
}

func writeResponse(w http.ResponseWriter, code int, resp interface{}) {
	enc := json.NewEncoder(w)
	w.WriteHeader(code)
	enc.Encode(resp)
}

func errorResponse(w http.ResponseWriter, code int, msg string) {
	writeResponse(w, http.StatusInternalServerError, &ErrorResponse{Msg: msg})
}
