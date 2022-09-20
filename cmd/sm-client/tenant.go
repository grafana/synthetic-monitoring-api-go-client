package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	smapi "github.com/grafana/synthetic-monitoring-api-go-client"
	"github.com/urfave/cli/v2"
)

func getTenantCommands() cli.Commands {
	return cli.Commands{
		&cli.Command{
			Name:   "get-api-token",
			Usage:  "get new Synthetic Monitoring API token",
			Action: getApiToken,
		},
		&cli.Command{
			Name:   "delete-api-token",
			Usage:  "delete Synthetic Monitoring API token",
			Action: deleteApiToken,
		},
		&cli.Command{
			Name:   "get",
			Usage:  "get Synthetic Monitoring tenant",
			Action: getTenant,
		},
		&cli.Command{
			Name:   "update",
			Usage:  "update Synthetic Monitoring tenant",
			Action: updateTenant,
		},
	}
}

func getApiToken(c *cli.Context) error {
	smClient := smapi.NewClient(c.String("sm-api-url"), "", nil)

	var newToken string

	token := c.String("sm-api-token")
	if token == "" {
		// We don't have a token, get one by calling the Install path.
		resp, err := smClient.Install(
			c.Context,
			c.Int64("grafana-instance-id"),
			c.Int64("metrics-instance-id"),
			c.Int64("logs-instance-id"),
			c.String("publisher-token"),
		)
		if err != nil {
			return fmt.Errorf("setting up Synthetic Monitoring tenant: %w", err)
		}

		newToken = resp.AccessToken
	} else {
		// We already have a token, get another one by calling the Create path.
		resp, err := smClient.CreateToken(c.Context)
		if err != nil {
			return fmt.Errorf("getting new Synthetic Monitoring tenant token: %w", err)
		}

		newToken = resp
	}

	fmt.Printf("token: %s\n", newToken)

	return nil
}

func deleteApiToken(c *cli.Context) error {
	token := c.String("sm-api-token")
	if token == "" {
		return cli.Exit("invalid API token", 1)
	}

	smClient := smapi.NewClient(c.String("sm-api-url"), token, nil)

	err := smClient.DeleteToken(c.Context)
	if err != nil {
		return fmt.Errorf("deleting token: %w", err)
	}

	return nil
}

func getTenant(c *cli.Context) error {
	smClient, cleanup, err := newClient(c)
	if err != nil {
		return err
	}

	defer func() { _ = cleanup(c.Context) }()

	tenant, err := smClient.GetTenant(c.Context)
	if err != nil {
		return fmt.Errorf("getting tenant: %w", err)
	}

	if done, err := outputJson(c, &tenant, "marshaling tenant"); err != nil || done {
		return err
	}

	w := newTabWriter(os.Stdout)
	fmt.Fprintf(w, "Id:\t%d\n", tenant.Id)
	fmt.Fprintf(w, "Org id:\t%d\n", tenant.OrgId)
	fmt.Fprintf(w, "Stack id:\t%d\n", tenant.StackId)
	if tenant.Reason != "" {
		fmt.Fprintf(w, "Status:\t%s (%s)\n", tenant.Status, tenant.Reason)
	} else {
		fmt.Fprintf(w, "Status:\t%s\n", tenant.Status)
	}
	fmt.Fprintf(w, "Metrics remote:\t%s, %s, %s\n", tenant.MetricsRemote.Name, tenant.MetricsRemote.Username, tenant.MetricsRemote.Url)
	fmt.Fprintf(w, "Events remote:\t%s, %s, %s\n", tenant.EventsRemote.Name, tenant.EventsRemote.Username, tenant.EventsRemote.Url)
	fmt.Fprintf(w, "Created:\t%s\n", formatSMTime(tenant.Created))
	fmt.Fprintf(w, "Modified:\t%s\n", formatSMTime(tenant.Modified))

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flushing output: %w", err)
	}

	return nil
}

func updateTenant(c *cli.Context) error {
	smClient, cleanup, err := newClient(c)
	if err != nil {
		return err
	}

	defer func() { _ = cleanup(c.Context) }()

	var in synthetic_monitoring.Tenant
	if err := readJsonArg(c.Args().First(), &in); err != nil {
		return err
	}

	out, err := smClient.UpdateTenant(c.Context, in)
	if err != nil {
		return fmt.Errorf("updating tenant: %w", err)
	}

	buf, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("marshaling tenant: %w", err)
	}

	fmt.Printf("%s\n", string(buf))

	return nil
}
