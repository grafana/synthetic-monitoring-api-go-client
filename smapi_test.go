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
	errBadRequest                 = errors.New("bad request")
	errCannotDecodeRequest        = errors.New("cannot decode request")
	errInvalidAuthorizationHeader = errors.New("no authorization header")
	errInvalidMethod              = errors.New("invalid method")
	errInvalidTenantID            = errors.New("invalid tenantId")
	errNotAuthorized              = errors.New("not authorized")
	errUnexpectedID               = errors.New("unexpected ID")
)

type stackInfo struct {
	id               int64
	metricInstanceID int64
	logInstanceID    int64
}

type orgInfo struct {
	id             int64
	adminToken     string
	publisherToken string
	tenant         tenantInfo
	metricInstance model.HostedInstance
	logInstance    model.HostedInstance
	stacks         []stackInfo
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

func (db db) findOrgByID(id int64) *orgInfo {
	for _, org := range db {
		if org.id == id {
			return &org
		}
	}

	return nil
}

// findOrgByToken finds the organization that corresponds to the given
// token.
func (db db) findOrgByToken(token string) *orgInfo {
	for _, org := range db {
		if org.adminToken == token {
			return &org
		}

		if org.publisherToken == token {
			return &org
		}
	}

	return nil
}

func (db db) findTenantByOrg(id int64) *tenantInfo {
	org := db.findOrgByID(id)
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

// orgs should be called to obtain a copy of the "database" so that the
// test can work against it.
//
// This guarantees that the database is not mutated between tests.
func orgs() db {
	return db{
		{
			id:             1000,
			adminToken:     "token-org-1000",
			publisherToken: "publisher-token-org-1000",
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
			stacks: []stackInfo{
				{
					id:               3,
					metricInstanceID: 1,
					logInstanceID:    2,
				},
			},
		},
	}
}

func (org orgInfo) validateStackByIds(id, metricsInstanceID, logsInstanceID int64) bool {
	for _, stack := range org.stacks {
		if id == stack.id &&
			metricsInstanceID == stack.metricInstanceID &&
			logsInstanceID == stack.logInstanceID {
			return true
		}
	}

	return false
}

type AuthTokenGetter interface {
	GetAuthToken(*http.Request) string
}

type InitRequest struct {
	model.RegistrationInitRequest
}

func (r *InitRequest) GetAuthToken(_ *http.Request) string {
	return r.AdminToken
}

type SaveRequest struct {
	model.RegistrationSaveRequest
}

func (r *SaveRequest) GetAuthToken(_ *http.Request) string {
	return r.AdminToken
}

type RegistrationInstallRequest struct {
	model.RegistrationInstallRequest
}

func (r *RegistrationInstallRequest) GetAuthToken(req *http.Request) string {
	authHeader := req.Header.Get("authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return ""
	}

	return strings.TrimPrefix(authHeader, "Bearer ")
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
		testcase := testcase
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
		resp, err := c.do(context.Background(), url, "/", false, nil, nil) //nolint:bodyclose
		validate(t, resp, err)
	})

	t.Run("invalid context", func(t *testing.T) {
		c := Client{client: http.DefaultClient}
		resp, err := c.do(nil, url, http.MethodGet, false, nil, nil) //nolint:staticcheck,bodyclose // passing nil context on purpose
		validate(t, resp, err)
	})

	t.Run("invalid url", func(t *testing.T) {
		c := Client{client: http.DefaultClient}
		resp, err := c.do(context.Background(), "://", http.MethodGet, false, nil, nil) //nolint:bodyclose
		validate(t, resp, err)
	})

	t.Run("context canceled", func(t *testing.T) {
		c := Client{client: http.DefaultClient}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()                                                     // cancel context now
		resp, err := c.do(ctx, url, http.MethodGet, false, nil, nil) //nolint:bodyclose
		validate(t, resp, err)
	})
}

func TestClientRegistrationInstall(t *testing.T) {
	url, mux, cleanup := newTestServer(t)
	defer cleanup()

	orgs := orgs()

	mux.Handle("/api/v1/register/install", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req RegistrationInstallRequest

		orgID, err := readPostRequest(orgs, w, r, &req, -1000)
		if err != nil {
			return
		}

		org := orgs.findOrgByID(orgID)
		if org == nil {
			errorResponse(w, http.StatusBadRequest, "org not found")

			return
		}

		if !org.validateStackByIds(req.StackID, req.MetricsInstanceID, req.LogsInstanceID) {
			errorResponse(w, http.StatusBadRequest, "invalid stack")

			return
		}

		resp := model.RegistrationInstallResponse{
			AccessToken: org.tenant.token,
			TenantInfo: &model.TenantDescription{
				ID:             org.tenant.id,
				MetricInstance: model.HostedInstance{ID: req.MetricsInstanceID},
				LogInstance:    model.HostedInstance{ID: req.LogsInstanceID},
			},
		}

		writeResponse(w, http.StatusOK, &resp)
	}))

	testOrg := orgs.findOrgByID(1000)
	require.NotNil(t, testOrg)
	require.NotEmpty(t, testOrg.stacks)

	testcases := map[string]struct {
		stackID           int64
		metricsInstanceID int64
		logsInstanceID    int64
		authToken         string
		shouldError       bool
	}{
		"org exists": {
			stackID:           testOrg.stacks[0].id,
			metricsInstanceID: testOrg.stacks[0].metricInstanceID,
			logsInstanceID:    testOrg.stacks[0].logInstanceID,
			authToken:         testOrg.publisherToken,
		},
		"token does not exist": {
			stackID:           100,
			metricsInstanceID: 200,
			logsInstanceID:    300,
			authToken:         "invalid",
			shouldError:       true,
		},
		"valid token, invalid stack": {
			stackID:           100,
			metricsInstanceID: 200,
			logsInstanceID:    300,
			authToken:         testOrg.publisherToken,
			shouldError:       true,
		},
	}

	for name, testcase := range testcases {
		testcase := testcase
		t.Run(name, func(t *testing.T) {
			c := NewClient(url, "", http.DefaultClient)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, err := c.Install(ctx, testcase.stackID, testcase.metricsInstanceID, testcase.logsInstanceID, testcase.authToken)

			if testcase.shouldError {
				require.Error(t, err)
				require.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
				require.Equal(t, resp.AccessToken, testOrg.tenant.token)
				require.NotNil(t, resp.TenantInfo, testOrg.tenant.token)
				require.Equal(t, resp.TenantInfo.ID, testOrg.tenant.id)
				require.Equal(t, resp.TenantInfo.MetricInstance.ID, testcase.metricsInstanceID)
				require.Equal(t, resp.TenantInfo.LogInstance.ID, testcase.logsInstanceID)
			}
		})
	}
}

func TestClientInit(t *testing.T) {
	url, mux, cleanup := newTestServer(t)
	defer cleanup()

	orgs := orgs()

	mux.Handle("/api/v1/register/init", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req InitRequest

		orgID, err := readPostRequest(orgs, w, r, &req, -1000)
		if err != nil {
			return
		}

		org := orgs.findOrgByID(orgID)

		resp := model.RegistrationInitResponse{
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

	testOrg := orgs.findOrgByID(1000)

	testcases := map[string]struct {
		orgID       int64
		shouldError bool
	}{
		"org exists":         {orgID: testOrg.id},
		"org does not exist": {orgID: 1, shouldError: true},
	}

	for name, testcase := range testcases {
		testcase := testcase
		t.Run(name, func(t *testing.T) {
			c := NewClient(url, "", http.DefaultClient)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var (
				token      string
				testTenant tenantInfo
			)

			testOrg := orgs.findOrgByID(testcase.orgID)
			if testOrg != nil {
				token = testOrg.adminToken
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
	orgs := orgs()
	testOrg := orgs.findOrgByID(1000)
	testTenant := testOrg.tenant

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/register/save", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req SaveRequest

		orgID, err := readPostRequest(orgs, w, r, &req, -1000)
		if err != nil {
			return
		}

		tenant := orgs.findTenantByOrg(orgID)
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

		resp := model.RegistrationSaveResponse{}

		writeResponse(w, http.StatusOK, &resp)
	}))

	c := NewClient(url, "", http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.Save(ctx, testOrg.adminToken, testOrg.metricInstance.ID, testOrg.logInstance.ID)
	require.NoError(t, err)
}

func TestAddProbe(t *testing.T) {
	orgs := orgs()
	testTenant := orgs.findTenantByOrg(1000)

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/probe/add", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req synthetic_monitoring.Probe
		tenantID, err := readPostRequest(orgs, w, r, &req, testTenant.id)
		if err != nil {
			return
		}
		if tenantID != testTenant.id {
			errorResponse(w, http.StatusExpectationFailed, fmt.Sprintf("expecting tenant ID %d, got %d", testTenant.id, tenantID))

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
		resp.Probe.TenantId = tenantID
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
	require.Empty(t, cmp.Diff(&probe, newProbe, ignoreIDField(), ignoreTenantIDField(), ignoreTimeFields()),
		"AddProbe mismatch (-want +got)")
	require.Equal(t, testTenant.probes[0].token, probeToken)
}

func TestUpdateProbe(t *testing.T) {
	orgs := orgs()

	testTenant := orgs.findTenantByOrg(1000)

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/probe/update", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := requireMethod(w, r, http.MethodPost); err != nil {
			return
		}

		var req synthetic_monitoring.Probe

		tenantID, err := readPostRequest(orgs, w, r, &req, testTenant.id)
		if err != nil {
			return
		}
		if tenantID != testTenant.id {
			errorResponse(w, http.StatusExpectationFailed, fmt.Sprintf("expecting tenant ID %d, got %d", testTenant.id, tenantID))

			return
		}

		found := false
		for _, probe := range testTenant.probes {
			if probe.id == req.Id {
				found = true

				break
			}
		}

		if !found {
			errorResponse(w, http.StatusNotFound, fmt.Sprintf("probe %d for tenant %d not found", req.Id, tenantID))

			return
		}

		var resp model.ProbeUpdateResponse
		resp.Probe.Id = req.Id
		resp.Probe.TenantId = tenantID
		resp.Probe.OnlineChange = 100
		resp.Probe.Created = 101
		resp.Probe.Modified = 102

		writeResponse(w, http.StatusOK, &resp)
	}))

	c := NewClient(url, testTenant.token, http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NotZero(t, len(testTenant.probes))

	probe := synthetic_monitoring.Probe{
		Id: testTenant.probes[0].id,
	}
	newProbe, err := c.UpdateProbe(ctx, probe)

	require.NoError(t, err)
	require.NotNil(t, newProbe)
	require.Equal(t, probe.Id, newProbe.Id)
	require.Equal(t, testTenant.id, newProbe.TenantId)
	require.Greater(t, newProbe.OnlineChange, float64(0))
	require.Greater(t, newProbe.Created, float64(0))
	require.Greater(t, newProbe.Modified, float64(0))
	require.Empty(t, cmp.Diff(&probe, newProbe, ignoreIDField(), ignoreTenantIDField(), ignoreTimeFields()),
		"UpdateProbe mismatch (-want +got)")
}

func TestResetProbeToken(t *testing.T) {
	orgs := orgs()

	testTenant := orgs.findTenantByOrg(1000)

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/probe/update", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := requireMethod(w, r, http.MethodPost); err != nil {
			return
		}

		var req synthetic_monitoring.Probe

		tenantID, err := readPostRequest(orgs, w, r, &req, testTenant.id)
		if err != nil {
			return
		}
		if tenantID != testTenant.id {
			errorResponse(w, http.StatusExpectationFailed, fmt.Sprintf("expecting tenant ID %d, got %d", testTenant.id, tenantID))

			return
		}

		found := false
		for key, values := range r.URL.Query() {
			if key == "reset-token" {
				if len(values) != 0 && values[0] != "" {
					errorResponse(w, http.StatusBadRequest, fmt.Sprintf(`"reset-token" should not have a value, got %q`, strings.Join(values, ",")))

					return
				}
				found = true

				break
			}
		}
		if !found {
			errorResponse(w, http.StatusBadRequest, `"reset-token" not found`)

			return
		}

		found = false
		for _, probe := range testTenant.probes {
			if probe.id == req.Id {
				found = true

				break
			}
		}

		if !found {
			errorResponse(w, http.StatusNotFound, fmt.Sprintf("probe %d for tenant %d not found", req.Id, tenantID))

			return
		}

		var resp model.ProbeUpdateResponse
		resp.Probe.Id = req.Id
		resp.Probe.TenantId = tenantID
		resp.Probe.OnlineChange = 100
		resp.Probe.Created = 101
		resp.Probe.Modified = 102
		resp.Token = []byte{0x20, 0x21, 0x22, 0x23}

		writeResponse(w, http.StatusOK, &resp)
	}))

	c := NewClient(url, testTenant.token, http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NotZero(t, len(testTenant.probes))

	probe := synthetic_monitoring.Probe{
		Id: testTenant.probes[0].id,
	}
	newProbe, newToken, err := c.ResetProbeToken(ctx, probe)

	require.NoError(t, err)
	require.NotNil(t, newProbe)
	require.NotNil(t, newToken)
	require.Equal(t, probe.Id, newProbe.Id)
	require.Equal(t, testTenant.id, newProbe.TenantId)
	require.Greater(t, newProbe.OnlineChange, float64(0))
	require.Greater(t, newProbe.Created, float64(0))
	require.Greater(t, newProbe.Modified, float64(0))
	require.Empty(t, cmp.Diff(&probe, newProbe, ignoreIDField(), ignoreTenantIDField(), ignoreTimeFields()),
		"UpdateProbe mismatch (-want +got)")
}

func TestListProbes(t *testing.T) {
	orgs := orgs()
	testTenant := orgs.findTenantByOrg(1000)
	testTenantID := testTenant.id
	probes := []synthetic_monitoring.Probe{
		{
			Id:        42,
			TenantId:  1,
			Name:      "probe-42",
			Latitude:  -33,
			Longitude: 151,
			Public:    true,
		},
		{
			Id:        43,
			TenantId:  testTenantID,
			Name:      "probe-43",
			Latitude:  10,
			Longitude: -84,
			Public:    false,
		},
	}

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/probe/list", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := requireMethod(w, r, http.MethodGet); err != nil {
			return
		}

		if _, err := requireAuth(orgs, w, r, testTenantID); err != nil {
			return
		}

		resp := probes

		writeResponse(w, http.StatusOK, &resp)
	}))

	c := NewClient(url, testTenant.token, http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	actualProbes, err := c.ListProbes(ctx)
	require.NoError(t, err)
	require.NotNil(t, actualProbes)
	require.ElementsMatch(t, probes, actualProbes)
}

func TestDeleteProbe(t *testing.T) {
	orgs := orgs()
	testTenant := orgs.findTenantByOrg(1000)
	testCheckID := int64(42)

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/probe/delete/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := requireMethod(w, r, http.MethodDelete); err != nil {
			return
		}

		if _, err := requireAuth(orgs, w, r, testTenant.id); err != nil {
			return
		}

		if err := requireID(w, r, testCheckID, "/api/v1/probe/delete/"); err != nil {
			return
		}

		resp := model.ProbeDeleteResponse{
			Msg:     "probe deleted",
			ProbeID: testCheckID,
		}

		writeResponse(w, http.StatusOK, &resp)
	}))

	c := NewClient(url, testTenant.token, http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.DeleteProbe(ctx, testCheckID)
	require.NoError(t, err)
}

func TestAddCheck(t *testing.T) {
	orgs := orgs()
	testTenant := orgs.findTenantByOrg(1000)
	testTenantID := testTenant.id
	testCheckID := int64(42)

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/check/add", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req synthetic_monitoring.Check
		tenantID, err := readPostRequest(orgs, w, r, &req, testTenantID)
		if err != nil {
			return
		}

		resp := req

		resp.Id = testCheckID
		resp.TenantId = tenantID
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
	require.Equal(t, testCheckID, newCheck.Id)
	require.Equal(t, testTenant.id, newCheck.TenantId)
	require.Greater(t, newCheck.Created, float64(0))
	require.Greater(t, newCheck.Modified, float64(0))
	require.Empty(t, cmp.Diff(&check, newCheck, ignoreIDField(), ignoreTenantIDField(), ignoreTimeFields()),
		"AddCheck mismatch (-want +got)")
}

func TestUpdateCheck(t *testing.T) {
	orgs := orgs()
	testTenant := orgs.findTenantByOrg(1000)
	testTenantID := testTenant.id
	testCheckID := int64(42)

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/check/update", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req synthetic_monitoring.Check
		tenantID, err := readPostRequest(orgs, w, r, &req, testTenantID)
		if err != nil {
			return
		}

		if req.Id != testCheckID {
			errorResponse(w, http.StatusBadRequest, fmt.Sprintf("expecting ID %d, got %d ", testCheckID, req.Id))

			return
		}

		if req.TenantId != tenantID {
			errorResponse(w, http.StatusBadRequest, fmt.Sprintf("expecting tenant ID %d, got %d ", tenantID, req.TenantId))

			return
		}

		resp := req

		resp.Created = 200
		resp.Modified = 201

		writeResponse(w, http.StatusOK, &resp)
	}))

	c := NewClient(url, testTenant.token, http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	check := synthetic_monitoring.Check{
		Id:               testCheckID,
		TenantId:         testTenantID,
		Frequency:        1000,
		Timeout:          500,
		Offset:           1,
		Target:           "target",
		Job:              "job",
		BasicMetricsOnly: true,
		Enabled:          true,
	}
	newCheck, err := c.UpdateCheck(ctx, check)

	require.NoError(t, err)
	require.NotNil(t, newCheck)
	require.Equal(t, testCheckID, newCheck.Id)
	require.Equal(t, testTenant.id, newCheck.TenantId)
	require.Greater(t, newCheck.Created, float64(0))
	require.Greater(t, newCheck.Modified, float64(0))
	require.Empty(t, cmp.Diff(&check, newCheck, ignoreIDField(), ignoreTenantIDField(), ignoreTimeFields()),
		"AddCheck mismatch (-want +got)")
}

func TestDeleteCheck(t *testing.T) {
	orgs := orgs()
	testTenant := orgs.findTenantByOrg(1000)
	testTenantID := testTenant.id
	testCheckID := int64(42)

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/check/delete/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := requireMethod(w, r, http.MethodDelete); err != nil {
			return
		}

		if _, err := requireAuth(orgs, w, r, testTenantID); err != nil {
			return
		}

		if err := requireID(w, r, testCheckID, "/api/v1/check/delete/"); err != nil {
			return
		}

		resp := model.CheckDeleteResponse{
			Msg:     "check deleted",
			CheckID: testCheckID,
		}

		writeResponse(w, http.StatusOK, &resp)
	}))

	c := NewClient(url, testTenant.token, http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.DeleteCheck(ctx, testCheckID)

	require.NoError(t, err)
}

func TestListChecks(t *testing.T) {
	orgs := orgs()
	testTenant := orgs.findTenantByOrg(1000)
	testTenantID := testTenant.id
	checks := []synthetic_monitoring.Check{
		{
			Id:       42,
			TenantId: testTenantID,
		},
		{
			Id:       43,
			TenantId: testTenantID,
		},
	}

	url, mux, cleanup := newTestServer(t)
	defer cleanup()
	mux.Handle("/api/v1/check/list", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := requireMethod(w, r, http.MethodGet); err != nil {
			return
		}

		if _, err := requireAuth(orgs, w, r, testTenantID); err != nil {
			return
		}

		resp := checks

		writeResponse(w, http.StatusOK, &resp)
	}))

	c := NewClient(url, testTenant.token, http.DefaultClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	actualChecks, err := c.ListChecks(ctx)
	require.NoError(t, err)
	require.NotNil(t, actualChecks)
	require.ElementsMatch(t, checks, actualChecks)
}

func newTestServer(t *testing.T) (string, *http.ServeMux, func()) {
	t.Helper()

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

		return errBadRequest
	}

	return nil
}

func requireID(w http.ResponseWriter, r *http.Request, expected int64, prefix string) error {
	str := strings.TrimPrefix(r.URL.Path, prefix)
	if actual, err := strconv.ParseInt(str, 10, 64); err != nil {
		errorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid ID: %s", str))

		return fmt.Errorf("cannot parse %q as int: %w", str, err)
	} else if actual != expected {
		errorResponse(w, http.StatusBadRequest, fmt.Sprintf("expecting ID %d, got %d ", expected, actual))

		return errUnexpectedID
	}

	return nil
}

func readPostRequest(orgs db, w http.ResponseWriter, r *http.Request, req interface{}, expectedTenantID int64) (int64, error) {
	if err := requireMethod(w, r, http.MethodPost); err != nil {
		return -1, errInvalidMethod
	}

	if r.Body == nil {
		errorResponse(w, http.StatusBadRequest, "invalid request")

		return -1, errBadRequest
	}
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(req); err != nil {
		errorResponse(w, http.StatusInternalServerError, "cannot decode request")

		return -1, errCannotDecodeRequest
	}

	if req, ok := req.(AuthTokenGetter); ok {
		org := orgs.findOrgByToken(req.GetAuthToken(r))
		if org == nil {
			errorResponse(w, http.StatusUnauthorized, "not authorized")

			return -1, errNotAuthorized
		}

		return org.id, nil
	}

	return requireAuth(orgs, w, r, expectedTenantID)
}

func requireAuth(orgs db, w http.ResponseWriter, r *http.Request, tenantID int64) (int64, error) {
	authHeader := r.Header.Get("authorization")
	if authHeader == "" {
		return 0, errInvalidAuthorizationHeader
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	actualTenant := orgs.findTenantByToken(token)
	if actualTenant == nil {
		errorResponse(w, http.StatusUnauthorized, "not authorized")

		return 0, errNotAuthorized
	}

	if actualTenant.id != tenantID {
		errorResponse(w, http.StatusExpectationFailed, fmt.Sprintf("expecinting tenant ID %d, got %d", tenantID, actualTenant.id))

		return 0, errInvalidTenantID
	}

	return tenantID, nil
}

func writeResponse(w http.ResponseWriter, code int, resp interface{}) {
	enc := json.NewEncoder(w)
	w.WriteHeader(code)
	_ = enc.Encode(resp)
}

func errorResponse(w http.ResponseWriter, code int, msg string) {
	writeResponse(w, code, &model.ErrorResponse{Msg: msg})
}
