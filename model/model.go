package model

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
)

const (
	InstanceTypePrometheus = "prometheus"
	InstanceTypeLogs       = "logs"
)

type ErrorResponse struct {
	Msg string `json:"msg,omitempty"`
	Err error  `json:"err,omitempty"`
}

type RegistrationInstallRequest struct {
	StackID           int64 `json:"stackId"`
	MetricsInstanceID int64 `json:"metricsInstanceId"`
	LogsInstanceID    int64 `json:"logsInstanceId"`
}

type RegistrationInstallResponse struct {
	AccessToken string             `json:"accessToken"`
	TenantInfo  *TenantDescription `json:"tenantInfo,omitempty"`
}

type TokenCreateResponse struct {
	Msg         string `json:"msg,omitempty"`
	AccessToken string `json:"accessToken"`
}

type TokenDeleteResponse struct {
	Msg string `json:"msg,omitempty"`
}

type TokenRefreshResponse struct {
	Msg         string `json:"msg,omitempty"`
	AccessToken string `json:"accessToken"`
}

type TokenValidateResponse struct {
	Msg     string `json:"msg,omitempty"`
	IsValid bool   `json:"isValid"`
}

type TenantDescription struct {
	ID             int64          `json:"id"`
	MetricInstance HostedInstance `json:"metricInstance"`
	LogInstance    HostedInstance `json:"logInstance"`
}

type HostedInstance struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type ProbeAddResponse struct {
	Probe synthetic_monitoring.Probe `json:"probe"`
	Token []byte                     `json:"token"`
}

type ProbeDeleteResponse struct {
	Msg     string `json:"msg"`
	ProbeID int64  `json:"probeId"`
}

type ProbeUpdateResponse struct {
	Probe synthetic_monitoring.Probe `json:"probe"`
	Token []byte                     `json:"token,omitempty"`
}

type CheckDeleteResponse struct {
	Msg     string `json:"msg"`
	CheckID int64  `json:"checkId"`
}

func (e *ErrorResponse) Error() string {
	switch {
	case e == nil:
		return ""

	case e.Err != nil:
		return fmt.Sprintf(`msg="%s" error="%s"`, e.Msg, e.Err.Error())

	case e.Msg != "":
		return fmt.Sprintf(`msg="%s"`, e.Msg)

	default:
		return ""
	}
}

func (e *ErrorResponse) MarshalJSON() ([]byte, error) {
	var resp struct {
		Msg string `json:"msg,omitempty"`
		Err string `json:"err,omitempty"`
	}

	if e != nil {
		resp.Msg = e.Msg

		if e.Err != nil {
			resp.Err = e.Err.Error()
		}
	}

	buf, err := json.Marshal(&resp)
	if err != nil {
		// This should never happen.
		return nil, fmt.Errorf("cannot marshal error: %w", err)
	}

	return buf, nil
}
