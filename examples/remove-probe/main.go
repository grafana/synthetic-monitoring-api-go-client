package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	smapi "github.com/grafana/synthetic-monitoring-api-go-client"
)

type config struct {
	ApiServerURL      string
	GrafanaInstanceID int64
	MetricsInstanceID int64
	LogsInstanceID    int64
	PublisherToken    string
	ProbeName         string
}

func main() {
	cfg, err := processFlags(os.Args[1:])

	if errors.Is(err, flag.ErrHelp) {
		os.Exit(0)
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "invalid arguments: %s\n", err.Error())
		os.Exit(1)
	}

	c := smapi.NewClient(cfg.ApiServerURL, "", http.DefaultClient)

	ctx := context.Background()

	installResp, err := c.Install(ctx, cfg.GrafanaInstanceID, cfg.MetricsInstanceID, cfg.LogsInstanceID, cfg.PublisherToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error calling install: %s\n", err.Error())
		return
	}

	// delete the token we created by calling c.Install above.
	defer c.DeleteToken(ctx)

	probe, err := findProbe(ctx, cfg.ProbeName, installResp.TenantInfo.ID, c)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot find probe: %s\n", err.Error())
		return
	}

	if err := removeProbeFromChecks(ctx, probe, c); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot remove probe from checks: %s\n", err.Error())
		return
	}
}

func processFlags(args []string) (config, error) {
	fs := flag.NewFlagSet("", flag.ContinueOnError)

	var cfg config

	fs.StringVar(&cfg.ApiServerURL, "api-server-url", "https://synthetic-monitoring-api.grafana.net", "URL to contact the API server")
	fs.Int64Var(&cfg.GrafanaInstanceID, "grafana-instance-id", 0, "grafana.com Grafana instance ID")
	fs.Int64Var(&cfg.MetricsInstanceID, "metrics-instance-id", 0, "grafana.com hosted metrics instance ID")
	fs.Int64Var(&cfg.LogsInstanceID, "logs-instance-id", 0, "grafana.com hosted logs instance ID")
	fs.StringVar(&cfg.PublisherToken, "publisher-token", "", "grafana.com publisher token")
	fs.StringVar(&cfg.ProbeName, "probe-name", "", "Synthetic Monitoring probe to remove from checks")

	switch err := fs.Parse(args); {
	case errors.Is(err, flag.ErrHelp):
		return cfg, err

	case err != nil:
		return cfg, fmt.Errorf("invalid arguments")
	}

	if cfg.ApiServerURL == "" {
		return cfg, fmt.Errorf("invalid API server URL: %s", cfg.ApiServerURL)
	}

	if cfg.GrafanaInstanceID <= 0 {
		return cfg, fmt.Errorf("invalid grafana instance id: %d", cfg.GrafanaInstanceID)
	}

	if cfg.MetricsInstanceID <= 0 {
		return cfg, fmt.Errorf("invalid metrics instance id: %d", cfg.MetricsInstanceID)
	}

	if cfg.LogsInstanceID <= 0 {
		return cfg, fmt.Errorf("invalid logs instance id: %d", cfg.LogsInstanceID)
	}

	if cfg.PublisherToken == "" {
		return cfg, fmt.Errorf(`invalid publisher token: "%s"`, cfg.PublisherToken)
	}

	if cfg.ProbeName == "" {
		return cfg, fmt.Errorf(`invalid probe name: "%s"`, cfg.ProbeName)
	}

	return cfg, nil
}

func findProbe(ctx context.Context, name string, tenantID int64, client *smapi.Client) (synthetic_monitoring.Probe, error) {
	existingProbes, err := client.ListProbes(ctx)
	if err != nil {
		return synthetic_monitoring.Probe{}, fmt.Errorf("listing probes: %w", err)
	}

	for _, p := range existingProbes {
		if strings.EqualFold(p.Name, name) && (p.TenantId == tenantID || p.Public) {
			return p, nil
		}
	}

	return synthetic_monitoring.Probe{}, fmt.Errorf(`Probe "%s" not found.`, name)
}

func removeProbeFromChecks(ctx context.Context, probe synthetic_monitoring.Probe, client *smapi.Client) error {
	checks, err := client.ListChecks(ctx)
	if err != nil {
		return fmt.Errorf("cannot list checks: %w", err)
	}

	for _, check := range checks {
		for i, checkProbeId := range check.Probes {
			if checkProbeId != probe.Id {
				continue
			}

			if i+1 < len(check.Probes) {
				copy(check.Probes[i:], check.Probes[i+1:])
			}
			if len(check.Probes) > 0 {
				check.Probes = check.Probes[:len(check.Probes)-1]
			}

			_, err := client.UpdateCheck(ctx, check)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error updating check: %s", err)
			}

			fmt.Printf("Removed probe %s (%d) from check with job %s, target %s\n", probe.Name, probe.Id, check.Job, check.Target)

			break
		}
	}

	return nil
}
