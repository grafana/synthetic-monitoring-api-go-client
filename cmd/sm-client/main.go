package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"text/tabwriter"

	smapi "github.com/grafana/synthetic-monitoring-api-go-client"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "sm-client",
		Usage: "Make requests to Synthetic Monitoring API",
		Flags: getGlobalFlags(),
		Commands: cli.Commands{
			&cli.Command{
				Name:        "tenant",
				Usage:       "tenant actions",
				Aliases:     []string{"tenants"},
				Subcommands: getTenantCommands(),
			},
			&cli.Command{
				Name:        "probe",
				Usage:       "probe actions",
				Aliases:     []string{"probes"},
				Subcommands: getProbeCommands(),
			},
			&cli.Command{
				Name:        "check",
				Usage:       "check actions",
				Aliases:     []string{"checks"},
				Subcommands: getCheckCommands(),
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func getGlobalFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:  "sm-api-url",
			Value: "https://synthetic-monitoring-api.grafana.net/",
			Usage: "base URL used to access the Synthetic Monitoring API server",
		},
		&cli.StringFlag{
			Name:    "sm-api-token",
			Value:   "",
			Usage:   "token used to access the Synthetic Monitoring API server",
			EnvVars: []string{"SM_API_TOKEN"},
		},
		&cli.Int64Flag{
			Name:  "grafana-instance-id",
			Value: 0,
			Usage: "Grafana Cloud's Grafana instance ID",
		},
		&cli.Int64Flag{
			Name:  "metrics-instance-id",
			Value: 0,
			Usage: "Grafana Cloud's metrics instance ID",
		},
		&cli.Int64Flag{
			Name:  "logs-instance-id",
			Value: 0,
			Usage: "Grafana Cloud's logs instance ID",
		},
		&cli.StringFlag{
			Name:    "publisher-token",
			Value:   "",
			Usage:   "Grafana Cloud publisher token",
			EnvVars: []string{"GRAFANA_PUBLISHER_TOKEN"},
		},
		&cli.BoolFlag{
			Name:  "json",
			Value: false,
			Usage: "output JSON",
		},
	}
}

func newClient(c *cli.Context) (*smapi.Client, func(context.Context) error, error) {
	token := c.String("sm-api-token")
	smClient := smapi.NewClient(c.String("sm-api-url"), token, nil)

	if token != "" {
		return smClient, func(context.Context) error { return nil }, nil
	}

	_, err := smClient.Install(
		c.Context,
		c.Int64("grafana-instance-id"),
		c.Int64("metrics-instance-id"),
		c.Int64("logs-instance-id"),
		c.String("publisher-token"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("setting up Synthetic Monitoring tenant: %w", err)
	}

	return smClient, smClient.DeleteToken, nil
}

func readJsonArg(arg string, dst interface{}) error {
	var buf []byte

	if len(arg) > 0 && arg[0] == '@' {
		fh, err := os.Open(arg[1:])
		if err != nil {
			return fmt.Errorf("opening input: %w", err)
		}
		defer func() { _ = fh.Close() }()

		buf, err = io.ReadAll(fh)
		if err != nil {
			return fmt.Errorf("reading input: %w", err)
		}
	} else {
		buf = []byte(arg)
	}

	if err := json.Unmarshal(buf, dst); err != nil {
		return fmt.Errorf("unmarshaling JSON input: %w", err)
	}

	return nil
}

func newTabWriter(w io.Writer) *tabwriter.Writer {
	const padding = 2

	return tabwriter.NewWriter(w, 0, 0, padding, ' ', 0)
}

func outputJson(c *cli.Context, v interface{}, errMsg string) (bool, error) {
	if !c.Bool("json") {
		return false, nil
	}

	enc := json.NewEncoder(c.App.Writer)

	if err := enc.Encode(v); err != nil {
		return true, fmt.Errorf("%s: %w", errMsg, err)
	}

	return true, nil
}
