package cli

import (
	"context"
	"fmt"

	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/urfave/cli/v3"
)

func GetTenantCommands(c TenantsClient) []*cli.Command {
	return []*cli.Command{
		{
			Name:   "get-api-token",
			Usage:  "get new Synthetic Monitoring API token",
			Action: c.getApiToken,
		},
		{
			Name:   "delete-api-token",
			Usage:  "delete Synthetic Monitoring API token",
			Action: c.deleteApiToken,
		},
		{
			Name:   "get",
			Usage:  "get Synthetic Monitoring tenant",
			Action: c.getTenant,
		},
		{
			Name:   "update",
			Usage:  "update Synthetic Monitoring tenant",
			Action: c.updateTenant,
		},
	}
}

type TenantsClient ServiceClient

func (c TenantsClient) getApiToken(ctx context.Context, cmd *cli.Command) error {
	smClient, cleanup, err := c.ClientBuilder(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(ctx) }()

	var newToken string

	if token := cmd.String("sm-api-token"); token == "" {
		// We don't have a token, get one by calling the Install path.
		resp, err := smClient.Install(
			ctx,
			cmd.Int64("grafana-instance-id"),
			cmd.Int64("metrics-instance-id"),
			cmd.Int64("logs-instance-id"),
			cmd.String("publisher-token"),
		)
		if err != nil {
			return fmt.Errorf("setting up Synthetic Monitoring tenant: %w", err)
		}

		newToken = resp.AccessToken
	} else {
		// We already have a token, get another one by calling the Create path.
		resp, err := smClient.CreateToken(ctx)
		if err != nil {
			return fmt.Errorf("getting new Synthetic Monitoring tenant token: %w", err)
		}

		newToken = resp
	}

	fmt.Printf("token: %s\n", newToken)

	return nil
}

func (c TenantsClient) deleteApiToken(ctx context.Context, cmd *cli.Command) error {
	token := cmd.String("sm-api-token")
	if token == "" {
		return cli.Exit("invalid API token", 1)
	}

	smClient, cleanup, err := c.ClientBuilder(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(ctx) }()

	err = smClient.DeleteToken(ctx)
	if err != nil {
		return fmt.Errorf("deleting token: %w", err)
	}

	return nil
}

func (c TenantsClient) getTenant(ctx context.Context, cmd *cli.Command) error {
	smClient, cleanup, err := c.ClientBuilder(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(ctx) }()

	tenant, err := smClient.GetTenant(ctx)
	if err != nil {
		return fmt.Errorf("getting tenant: %w", err)
	}

	return c.printTenant(ctx, cmd, tenant)
}

func (c TenantsClient) updateTenant(ctx context.Context, cmd *cli.Command) error {
	smClient, cleanup, err := c.ClientBuilder(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(ctx) }()

	var in synthetic_monitoring.Tenant
	if err := readJsonArg(cmd.Args().First(), &in); err != nil {
		return err
	}

	out, err := smClient.UpdateTenant(ctx, in)
	if err != nil {
		return fmt.Errorf("updating tenant: %w", err)
	}

	return c.printTenant(ctx, cmd, out)
}

func (c TenantsClient) printTenant(ctx context.Context, cmd *cli.Command, tenant *synthetic_monitoring.Tenant) error {
	jsonWriter := c.JsonWriterBuilder(cmd)
	if done, err := jsonWriter(tenant, "marshaling tenant"); err != nil || done {
		return err
	}

	w := c.TabWriterBuilder(cmd)
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
