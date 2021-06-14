package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"
	"text/template"

	smapi "github.com/grafana/synthetic-monitoring-api-go-client"
)

type config struct {
	ApiServerURL      string
	GrafanaInstanceID int64
	MetricsInstanceID int64
	LogsInstanceID    int64
	PublisherToken    string
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

	// Get a new access token.
	_, err = c.Install(ctx, cfg.GrafanaInstanceID, cfg.MetricsInstanceID, cfg.LogsInstanceID, cfg.PublisherToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error calling install: %s\n", err.Error())
		return
	}

	// Delete the access token when we are done.
	defer c.DeleteToken(ctx)

	err = listChecks(ctx, c, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing checks: %s\n", err.Error())
		return
	}
}

const checkListTempl = `Id	Job	Target	Enabled	Number of probes
------	------	------	------	------
{{range .}}{{.Id}}	{{.Job}}	{{.Target}}	{{if .Enabled}}✓{{end}}	{{len .Probes}}
{{end}}`

func listChecks(ctx context.Context, client *smapi.Client, w io.Writer) error {
	checks, err := client.ListChecks(ctx)
	if err != nil {
		return fmt.Errorf("cannot list checks: %w", err)
	}

	tw := tabwriter.NewWriter(w, 8, 8, 8, ' ', 0)

	t := template.New("test")
	t, _ = t.Parse(checkListTempl)

	if err := t.Execute(tw, checks); err != nil {
		return err
	}

	tw.Flush()

	return nil
}

func processFlags(args []string) (config, error) {
	fs := flag.NewFlagSet("", flag.ContinueOnError)

	var cfg config

	fs.StringVar(&cfg.ApiServerURL, "api-server-url", "https://synthetic-monitoring-api.grafana.net", "URL to contact the API server")
	fs.Int64Var(&cfg.GrafanaInstanceID, "grafana-instance-id", 0, "grafana.com Grafana instance ID")
	fs.Int64Var(&cfg.MetricsInstanceID, "metrics-instance-id", 0, "grafana.com hosted metrics instance ID")
	fs.Int64Var(&cfg.LogsInstanceID, "logs-instance-id", 0, "grafana.com hosted logs instance ID")
	fs.StringVar(&cfg.PublisherToken, "publisher-token", "", "grafana.com publisher token")

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

	return cfg, nil
}
