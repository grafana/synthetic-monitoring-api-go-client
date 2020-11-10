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

type orgInfo struct {
	id             int64
	token          string
	tenant         tenantInfo
	metricInstance model.HostedInstance
	logInstance    model.HostedInstance
}

type tenantInfo struct {
	id     int64
	token  string
	probes []probeInfo
}

type probeInfo struct {
	id    int64
	token []byte
}

type db []orgInfo

func (db db) findOrgById(id int64) *orgInfo {
	for _, org := range db {
		if org.id == id {
			return &org
		}
	}

	return nil
}

func (db db) findOrgByToken(token string) *orgInfo {
	for _, org := range db {
		if org.token == token {
			return &org
		}
	}

	return nil
}

func (db db) findTenantByOrg(id int64) *tenantInfo {
	org := db.findOrgById(id)
	if org != nil {
		return &org.tenant
	}

	return nil
}

func (db db) findTenantByToken(token string) *tenantInfo {
	for _, org := range db {
		if org.tenant.token == token {
			return &org.tenant
		}
	}

	return nil
}

func (db db) findInstancesByOrg(id int64) []model.HostedInstance {
	for _, org := range db {
		if org.id == id {
			return []model.HostedInstance{
				org.metricInstance,
				org.logInstance,
			}
		}
	}

	return nil
}

var orgs = db{
	{
		id:    1000,
		token: "token-org-1000",
		tenant: tenantInfo{
			id:    2000,
			token: "token-tenant-2000",
			probes: []probeInfo{
				{
					id:    1,
					token: []byte{0x01, 0x02, 0x03, 0x04},
				},
			},
		},
		metricInstance: model.HostedInstance{
			ID:   1,
			Type: model.InstanceTypePrometheus,
			Name: "org-1000-prom",
			URL:  "https://prometheus.grafana",
		},
		logInstance: model.HostedInstance{
			ID:   2,
			Type: model.InstanceTypeLogs,
			Name: "org-1000-logs",
			URL:  "https://logs.grafana",
		},
	},
}

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

func TestNewClient(t *testing.T) {
	url, _, cleanup := newTestServer(t)
	defer cleanup()

	testcases := map[string]struct {
		url         string
		accessToken string
		client      *http.Client
	}{
		"trivial": {
			url: url,
		},
		"extra slash": {
			url: url + "/",
		},
		"access token": {
			url:         url,
			accessToken: "123",
		},
		"default http client": {
			url:    url,
			client: http.DefaultClient,
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			c := NewClient(testcase.url, testcase.accessToken, testcase.client)

			require.NotNil(t, c)
			require.NotNil(t, c.client)
			if testcase.client != nil {
				require.Equal(t, testcase.client, c.client)
			}
			require.Equal(t, c.accessToken, testcase.accessToken)
			require.Equal(t, c.baseURL, url+"/api/v1")
		})
	}
}

// TestClientDo tests the "do" method of the API client in order to make
// sure that it does handle errors correctly.
func TestClientDo(t *testing.T) {
	url, _, cleanup := newTestServer(t)
	defer cleanup()

	validate := func(t *testing.T, resp *http.Response, err error) {
		t.Helper()

		require.Error(t, err)
		require.Nil(t, resp)

		if err == nil && resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}

	t.Run("invalid method", func(t *testing.T) {
		c := Client{client: http.DefaultClient}
		resp, err := c.do(context.Background(), url, "/", nil, nil) //nolint:bodyclose
		validate(t, resp, err)
	})

	t.Run("invalid context", func(t *testing.T) {
		c := Client{client: http.DefaultClient}
		resp, err := c.do(nil, url, http.MethodGet, nil, nil) //nolint:staticcheck,bodyclose // passing nil context on purpose
		validate(t, resp, err)
	})

	t.Run("invalid url", func(t *testing.T) {
		c := Client{client: http.DefaultClient}
		resp, err := c.do(context.Background(), "://", http.MethodGet, nil, nil) //nolint:bodyclose
		validate(t, resp, err)
	})

	t.Run("context canceled", func(t *testing.T) {
		c := Client{client: http.DefaultClient}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()                                              // cancel context now
		resp, err := c.do(ctx, url, http.MethodGet, nil, nil) //nolint:bodyclose
		validate(t, resp, err)
	})
}

func TestClientInit(t *testing.T) {
	url, mux, cleanup := newTestServer(t)
	defer cleanup()

	mux.Handle("/api/v1/register/init", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req InitRequest

		orgId, err := readPostRequest(w, r, &req, -1000)
		if err != nil {
			return
		}

		org := orgs.findOrgById(orgId)

		resp := model.InitResponse{
			AccessToken: org.tenant.token,
			TenantInfo: &model.TenantDescription{
				ID:             org.tenant.id,
				MetricInstance: model.HostedInstance{},
				LogInstance:    model.HostedInstance{},
			},
			Instances: orgs.findInstancesByOrg(org.id),
		}

		writeResponse(w, http.StatusOK, &resp)
	}))

	testOrg := orgs.findOrgById(1000)

	testcases := map[string]struct {
		orgId       int64
		shouldError bool
	}{
		"org exists":         {orgId: testOrg.id},
		"org does not exist": {orgId: 1, shouldError: true},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			c := NewClient(url, "", http.DefaultClient)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var (
				token      string
				testTenant tenantInfo
			)

			testOrg := orgs.findOrgById(testcase.orgId)
			if testOrg != nil {
				token = testOrg.token
				testTenant = testOrg.tenant
			}

			resp, err := c.Init(ctx, token)

			if testcase.shouldError {
				require.Error(t, err)
				require.Nil(t, resp)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Equal(t, testTenant.token, resp.AccessToken)
			require.NotNil(t, resp.TenantInfo)
			require.Equal(t, testTenant.id, resp.TenantInfo.ID)
			require.NotNil(t, resp.Instances)
			require.ElementsMatch(t, orgs.findInstancesByOrg(testOrg.id), resp.Instances)
			require.Equal(t, resp.AccessToken, c.accessToken, "client access token should be set after successful init call")
		})
	}
}

func TestClientSave(t *testing.T) {
	testOrg := orgs.findOrgById(1000)
	testTenant := testOrg.tenant

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/register/save", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req SaveRequest

		orgId, err := readPostRequest(w, r, &req, -1000)
		if err != nil {
			return
		}

		tenant := orgs.findTenantByOrg(orgId)
		if tenant == nil {
			errorResponse(w, http.StatusInternalServerError, "cannot get tenant")
			return
		}
		if tenant.id != testTenant.id {
			errorResponse(w, http.StatusExpectationFailed, fmt.Sprintf("expecting tenant ID %d, got %d", testTenant.id, tenant.id))
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

	c := NewClient(url, "", http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.Save(ctx, testOrg.token, testOrg.metricInstance.ID, testOrg.logInstance.ID)
	require.NoError(t, err)
}

func TestAddProbe(t *testing.T) {
	testTenant := orgs.findTenantByOrg(1000)

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/probe/add", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req synthetic_monitoring.Probe
		tenantId, err := readPostRequest(w, r, &req, testTenant.id)
		if err != nil {
			return
		}
		if tenantId != testTenant.id {
			errorResponse(w, http.StatusExpectationFailed, fmt.Sprintf("expecting tenant ID %d, got %d", testTenant.id, tenantId))
			return
		}

		if len(testTenant.probes) < 1 {
			errorResponse(w, http.StatusInternalServerError, "no probes for this tenant")
			return
		}

		// TODO(mem): how to handle multiple probes?

		resp := model.ProbeAddResponse{
			Token: testTenant.probes[0].token,
			Probe: req,
		}

		resp.Probe.Id = testTenant.probes[0].id
		resp.Probe.TenantId = tenantId
		resp.Probe.OnlineChange = 100
		resp.Probe.Created = 101
		resp.Probe.Modified = 102

		writeResponse(w, http.StatusOK, &resp)
	}))

	c := NewClient(url, testTenant.token, http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	probe := synthetic_monitoring.Probe{}
	newProbe, probeToken, err := c.AddProbe(ctx, probe)

	require.NoError(t, err)
	require.NotNil(t, newProbe)
	require.NotZero(t, newProbe.Id)
	require.Equal(t, testTenant.id, newProbe.TenantId)
	require.Greater(t, newProbe.OnlineChange, float64(0))
	require.Greater(t, newProbe.Created, float64(0))
	require.Greater(t, newProbe.Modified, float64(0))
	require.Empty(t, cmp.Diff(&probe, newProbe, ignoreIdField, ignoreTenantIdField, ignoreTimeFields),
		"AddProbe mismatch (-want +got)")
	require.Equal(t, testTenant.probes[0].token, probeToken)
}

func TestDeleteProbe(t *testing.T) {
	testTenant := orgs.findTenantByOrg(1000)
	testCheckId := int64(42)

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/probe/delete/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := requireMethod(w, r, http.MethodDelete); err != nil {
			return
		}

		if _, err := requireAuth(w, r, testTenant.id); err != nil {
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

	c := NewClient(url, testTenant.token, http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.DeleteProbe(ctx, testCheckId)
	require.NoError(t, err)
}

func TestAddCheck(t *testing.T) {
	testTenant := orgs.findTenantByOrg(1000)
	testTenantId := testTenant.id
	testCheckId := int64(42)

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/check/add", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req synthetic_monitoring.Check
		tenantId, err := readPostRequest(w, r, &req, testTenantId)
		if err != nil {
			return
		}

		resp := req

		resp.Id = testCheckId
		resp.TenantId = tenantId
		resp.Created = 200
		resp.Modified = 201

		writeResponse(w, http.StatusOK, &resp)
	}))

	c := NewClient(url, testTenant.token, http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	check := synthetic_monitoring.Check{}
	newCheck, err := c.AddCheck(ctx, check)

	require.NoError(t, err)
	require.NotNil(t, newCheck)
	require.Equal(t, testCheckId, newCheck.Id)
	require.Equal(t, testTenant.id, newCheck.TenantId)
	require.Greater(t, newCheck.Created, float64(0))
	require.Greater(t, newCheck.Modified, float64(0))
	require.Empty(t, cmp.Diff(&check, newCheck, ignoreIdField, ignoreTenantIdField, ignoreTimeFields),
		"AddCheck mismatch (-want +got)")
}

func TestDeleteCheck(t *testing.T) {
	testTenant := orgs.findTenantByOrg(1000)
	testTenantId := testTenant.id
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

	c := NewClient(url, testTenant.token, http.DefaultClient)

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

func readPostRequest(w http.ResponseWriter, r *http.Request, req interface{}, expectedTenantId int64) (int64, error) {
	if err := requireMethod(w, r, http.MethodPost); err != nil {
		return -1, errors.New("invalid method")
	}

	if r.Body == nil {
		errorResponse(w, http.StatusBadRequest, "invalid request")
		return -1, errors.New("invalid request")
	}
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	err := dec.Decode(req)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "cannot decode request")
		return -1, errors.New("cannot decode request")
	}

	if req, ok := req.(AdminTokenGetter); ok {
		org := orgs.findOrgByToken(req.GetAdminToken())
		if org == nil {
			errorResponse(w, http.StatusUnauthorized, "not authorized")
			return -1, errors.New("not authorized")
		}
		return org.id, nil
	}

	return requireAuth(w, r, expectedTenantId)
}

func requireAuth(w http.ResponseWriter, r *http.Request, tenantId int64) (int64, error) {
	authHeader := r.Header.Get("authorization")
	if authHeader == "" {
		return 0, errors.New("no authorization header")
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	actualTenant := orgs.findTenantByToken(token)
	if actualTenant == nil {
		errorResponse(w, http.StatusUnauthorized, "not authorized")
		return 0, errors.New("not authorized")
	}

	if actualTenant.id != tenantId {
		errorResponse(w, http.StatusExpectationFailed, fmt.Sprintf("expecinting tenant ID %d, got %d", tenantId, actualTenant.id))
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
