package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	smapi "github.com/grafana/synthetic-monitoring-api-go-client"
	smCli "github.com/grafana/synthetic-monitoring-api-go-client/cli"
	"github.com/urfave/cli/v3"
)

func main() {
	checksClient := smCli.ChecksClient{
		ClientBuilder:     newClient,
		JsonWriterBuilder: newJsonWriter,
		TabWriterBuilder:  newTabWriter,
	}
	probesClient := smCli.ProbesClient{
		ClientBuilder:     newClient,
		JsonWriterBuilder: newJsonWriter,
		TabWriterBuilder:  newTabWriter,
	}
	tenantsClient := smCli.TenantsClient{
		ClientBuilder:     newClient,
		JsonWriterBuilder: newJsonWriter,
		TabWriterBuilder:  newTabWriter,
	}

	app := &cli.Command{
		Name:  "sm-client",
		Usage: "Make requests to Synthetic Monitoring API",
		Flags: getGlobalFlags(),
		Commands: []*cli.Command{
			{
				Name:     "tenant",
				Usage:    "tenant actions",
				Aliases:  []string{"tenants"},
				Commands: smCli.GetTenantCommands(tenantsClient),
			},
			{
				Name:     "probe",
				Usage:    "probe actions",
				Aliases:  []string{"probes"},
				Commands: smCli.GetProbeCommands(probesClient),
			},
			{
				Name:     "check",
				Usage:    "check actions",
				Aliases:  []string{"checks"},
				Commands: smCli.GetCheckCommands(checksClient),
			},
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
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
			Sources: cli.EnvVars("SM_API_TOKEN"),
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
			Sources: cli.EnvVars("GRAFANA_PUBLISHER_TOKEN"),
		},
		&cli.BoolFlag{
			Name:  "json",
			Value: false,
			Usage: "output JSON",
		},
	}
}

func newClient(cmd *cli.Command) (*smapi.Client, func(context.Context) error, error) {
	token := cmd.String("sm-api-token")
	smClient := smapi.NewClient(cmd.String("sm-api-url"), token, nil)

	if token != "" {
		return smClient, func(context.Context) error { return nil }, nil
	}

	_, err := smClient.Install(
		context.Background(),
		cmd.Int64("grafana-instance-id"),
		cmd.Int64("metrics-instance-id"),
		cmd.Int64("logs-instance-id"),
		cmd.String("publisher-token"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("setting up Synthetic Monitoring tenant: %w", err)
	}

	return smClient, smClient.DeleteToken, nil
}

func newTabWriter(cmd *cli.Command) smCli.WriteFlusher {
	const padding = 2

	return tabwriter.NewWriter(cmd.Root().Writer, 0, 0, padding, ' ', 0)
}

func newJsonWriter(cmd *cli.Command) func(interface{}, string) (bool, error) {
	if !cmd.Bool("json") {
		return func(interface{}, string) (bool, error) {
			return false, nil
		}
	}

	return func(value interface{}, errMsg string) (bool, error) {
		enc := json.NewEncoder(cmd.Root().Writer)

		if err := enc.Encode(value); err != nil {
			return true, fmt.Errorf("%s: %w", errMsg, err)
		}

		return true, nil
	}
}
