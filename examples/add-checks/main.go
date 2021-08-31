package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	smapi "github.com/grafana/synthetic-monitoring-api-go-client"
	"github.com/rs/zerolog"
)

type cfg struct {
	apiAccessToken    string
	apiServerURL      string
	grafanaInstanceID int64
	logsInstanceID    int64
	metricsInstanceID int64
	publisherToken    string
	removeAllChecks   bool
}

func (c cfg) Validate() error {
	if c.apiServerURL == "" {
		return fmt.Errorf("invalid API server URL: %q", c.apiServerURL)
	}

	return nil
}

func main() {
	logger := zerolog.New(os.Stdout)

	fs := flag.NewFlagSet("", flag.ContinueOnError)

	var cfg cfg

	fs.StringVar(&cfg.apiAccessToken, "api-access-token", "", "existing API access token")
	fs.StringVar(&cfg.apiServerURL, "api-server-url", "", "URL to contact the API server")
	fs.Int64Var(&cfg.grafanaInstanceID, "grafana-instance-id", 0, "grafana.com Grafana instance ID")
	fs.Int64Var(&cfg.logsInstanceID, "logs-instance-id", 0, "grafana.com hosted logs instance ID")
	fs.Int64Var(&cfg.metricsInstanceID, "metrics-instance-id", 0, "grafana.com hosted metrics instance ID")
	fs.StringVar(&cfg.publisherToken, "publisher-token", "", "grafana.com publisher token")
	fs.BoolVar(&cfg.removeAllChecks, "remove-checks", false, "remove existing checks")

	switch err := fs.Parse(os.Args[1:]); {
	case errors.Is(err, flag.ErrHelp):
		logger.Error().Err(err).Msg("invalid argument")
		fs.Usage()
		return

	case err != nil:
		logger.Error().Err(err).Msg("invalid argument")
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		logger.Error().Err(err).Send()
		fs.Usage()
		os.Exit(1)
	}

	logger = logger.With().
		Int64("grafana-instance-id", cfg.grafanaInstanceID).
		Int64("metrics-instance-id", cfg.metricsInstanceID).
		Int64("logs-instance-id", cfg.logsInstanceID).
		Str("publisher-token", cfg.publisherToken).
		Str("api-server-url", cfg.apiServerURL).
		Logger()

	ctx := context.Background()

	c, cleanup, tenantID, err := getClient(ctx, cfg, logger)
	if err != nil {
		logger.Error().Err(err).Msg("cannot get client")
		return
	}
	defer cleanup()

	logger = logger.With().Int64("tenant_id", tenantID).Logger()

	if cfg.removeAllChecks {
		if err := removeAllChecks(ctx, c, logger); err != nil {
			logger.Error().Err(err).Msg("removing existing checks")
			return
		}
	}

	if err := addChecks(ctx, c, logger); err != nil {
		logger.Error().Err(err).Msg("adding checks")
		return
	}
}

func getClient(ctx context.Context, cfg cfg, logger zerolog.Logger) (*smapi.Client, func(), int64, error) {
	var (
		c        *smapi.Client
		cleanup  func()
		tenantID int64
	)

	if cfg.apiAccessToken != "" {
		c = smapi.NewClient(cfg.apiServerURL, cfg.apiAccessToken, http.DefaultClient)

		cleanup = func() {}

		tenant, err := c.GetTenant(ctx)
		if err != nil {
			logger.Error().Err(err).Msg("cannot get tenant")
			return nil, nil, 0, err
		}

		tenantID = tenant.Id
	} else {
		c = smapi.NewClient(cfg.apiServerURL, "", http.DefaultClient)

		installResp, err := c.Install(ctx, cfg.grafanaInstanceID, cfg.metricsInstanceID, cfg.logsInstanceID, cfg.publisherToken)
		if err != nil {
			logger.Error().Err(err).Msg("calling install")
			return nil, nil, 0, err
		}

		cleanup = func() { _ = c.DeleteToken(ctx) }

		tenantID = installResp.TenantInfo.ID
	}

	return c, cleanup, tenantID, nil
}

func removeAllChecks(ctx context.Context, client *smapi.Client, logger zerolog.Logger) error {
	checks, err := client.ListChecks(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("listing checks")
		return err
	}

	for _, check := range checks {
		err := client.DeleteCheck(ctx, check.Id)
		if err != nil {
			return err
		}
	}

	return nil
}

func addChecks(ctx context.Context, client *smapi.Client, logger zerolog.Logger) error {
	probes, err := client.ListProbes(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("listing probes")
		return err
	}

	probeIDs := make([]int64, len(probes))
	for i, p := range probes {
		probeIDs[i] = p.Id
	}

	for _, check := range getTestChecks(1, probeIDs) {
		c, err := client.AddCheck(ctx, check)
		if err != nil {
			logger.Error().Err(err).Msg("adding check")
			continue
		}

		if c != nil {
			logger.Info().Int64("check_id", c.Id).Msg("added check")
		}
	}

	return nil
}

func getTestChecks(groupID int, probeIDs []int64) []synthetic_monitoring.Check {
	checkConfigs := []struct {
		target           string
		job              string
		basicMetricsOnly bool
		settings         synthetic_monitoring.CheckSettings
	}{
		{
			target:           "127.0.0.1",
			job:              "ping-ipv4-basic",
			basicMetricsOnly: true,
			settings: synthetic_monitoring.CheckSettings{
				Ping: &synthetic_monitoring.PingSettings{
					IpVersion: synthetic_monitoring.IpVersion_V4,
				},
			},
		},
		{
			target:           "::1",
			job:              "ping-ipv6-basic",
			basicMetricsOnly: true,
			settings: synthetic_monitoring.CheckSettings{
				Ping: &synthetic_monitoring.PingSettings{
					IpVersion: synthetic_monitoring.IpVersion_V6,
				},
			},
		},
		{
			target:           "127.0.0.1",
			job:              "ping-ipv4",
			basicMetricsOnly: false,
			settings: synthetic_monitoring.CheckSettings{
				Ping: &synthetic_monitoring.PingSettings{
					IpVersion: synthetic_monitoring.IpVersion_V4,
				},
			},
		},
		{
			target:           "::1",
			job:              "ping-ipv6",
			basicMetricsOnly: false,
			settings: synthetic_monitoring.CheckSettings{
				Ping: &synthetic_monitoring.PingSettings{
					IpVersion: synthetic_monitoring.IpVersion_V6,
				},
			},
		},

		// http
		{
			target:           "http://google.com/",
			job:              "http-ipv4-basic",
			basicMetricsOnly: true,
			settings: synthetic_monitoring.CheckSettings{
				Http: &synthetic_monitoring.HttpSettings{
					IpVersion: synthetic_monitoring.IpVersion_V4,
				},
			},
		},
		{
			target:           "https://google.com/",
			job:              "https-ipv4-basic",
			basicMetricsOnly: true,
			settings: synthetic_monitoring.CheckSettings{
				Http: &synthetic_monitoring.HttpSettings{
					IpVersion: synthetic_monitoring.IpVersion_V4,
				},
			},
		},
		{
			target:           "http://google.com/",
			job:              "http-ipv4",
			basicMetricsOnly: false,
			settings: synthetic_monitoring.CheckSettings{
				Http: &synthetic_monitoring.HttpSettings{
					IpVersion: synthetic_monitoring.IpVersion_V4,
				},
			},
		},
		{
			target:           "https://google.com/",
			job:              "https-ipv4",
			basicMetricsOnly: false,
			settings: synthetic_monitoring.CheckSettings{
				Http: &synthetic_monitoring.HttpSettings{
					IpVersion: synthetic_monitoring.IpVersion_V4,
				},
			},
		},

		// dns
		{
			target:           "google.com",
			job:              "dns-ipv4-basic",
			basicMetricsOnly: true,
			settings: synthetic_monitoring.CheckSettings{
				Dns: &synthetic_monitoring.DnsSettings{
					IpVersion:  synthetic_monitoring.IpVersion_V4,
					Server:     "dns.google",
					RecordType: synthetic_monitoring.DnsRecordType_A,
				},
			},
		},
		{
			target:           "google.com",
			job:              "dns-ipv4",
			basicMetricsOnly: false,
			settings: synthetic_monitoring.CheckSettings{
				Dns: &synthetic_monitoring.DnsSettings{
					IpVersion:  synthetic_monitoring.IpVersion_V4,
					Server:     "dns.google",
					RecordType: synthetic_monitoring.DnsRecordType_A,
				},
			},
		},

		// tcp
		{
			target:           "127.0.0.1:22",
			job:              "tcp-ipv4-basic",
			basicMetricsOnly: true,
			settings: synthetic_monitoring.CheckSettings{
				Tcp: &synthetic_monitoring.TcpSettings{
					IpVersion: synthetic_monitoring.IpVersion_V4,
				},
			},
		},
		{
			target:           "127.0.0.1:22",
			job:              "tcp-ipv4",
			basicMetricsOnly: false,
			settings: synthetic_monitoring.CheckSettings{
				Tcp: &synthetic_monitoring.TcpSettings{
					IpVersion: synthetic_monitoring.IpVersion_V4,
				},
			},
		},
	}

	checks := make([]synthetic_monitoring.Check, 0, len(checkConfigs))

	for _, cfg := range checkConfigs {
		checks = append(checks, synthetic_monitoring.Check{
			Target:           cfg.target,
			Job:              fmt.Sprintf("%s-%d", cfg.job, groupID),
			Frequency:        10000,
			Timeout:          2000,
			Enabled:          true,
			Probes:           probeIDs,
			Settings:         cfg.settings,
			BasicMetricsOnly: cfg.basicMetricsOnly,
		})
	}

	return checks
}
