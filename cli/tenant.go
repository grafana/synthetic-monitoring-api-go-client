package cli

import (
	"fmt"

	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/urfave/cli/v2"
)

func GetTenantCommands(c TenantsClient) cli.Commands {
	return cli.Commands{
		&cli.Command{
			Name:   "get-api-token",
			Usage:  "get new Synthetic Monitoring API token",
			Action: c.getApiToken,
		},
		&cli.Command{
			Name:   "delete-api-token",
			Usage:  "delete Synthetic Monitoring API token",
			Action: c.deleteApiToken,
		},
		&cli.Command{
			Name:   "get",
			Usage:  "get Synthetic Monitoring tenant",
			Action: c.getTenant,
		},
		&cli.Command{
			Name:   "update",
			Usage:  "update Synthetic Monitoring tenant",
			Action: c.updateTenant,
		},
	}
}

type TenantsClient ServiceClient

func (c TenantsClient) getApiToken(ctx *cli.Context) error {
	smClient, cleanup, err := c.ClientBuilder(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(ctx.Context) }()

	var newToken string

	if token := ctx.String("sm-api-token"); token == "" {
		// We don't have a token, get one by calling the Install path.
		resp, err := smClient.Install(
			ctx.Context,
			ctx.Int64("grafana-instance-id"),
			ctx.Int64("metrics-instance-id"),
			ctx.Int64("logs-instance-id"),
			ctx.String("publisher-token"),
		)
		if err != nil {
			return fmt.Errorf("setting up Synthetic Monitoring tenant: %w", err)
		}

		newToken = resp.AccessToken
	} else {
		// We already have a token, get another one by calling the Create path.
		resp, err := smClient.CreateToken(ctx.Context)
		if err != nil {
			return fmt.Errorf("getting new Synthetic Monitoring tenant token: %w", err)
		}

		newToken = resp
	}

	fmt.Printf("token: %s\n", newToken)

	return nil
}

func (c TenantsClient) deleteApiToken(ctx *cli.Context) error {
	token := ctx.String("sm-api-token")
	if token == "" {
		return cli.Exit("invalid API token", 1)
	}

	smClient, cleanup, err := c.ClientBuilder(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(ctx.Context) }()

	err = smClient.DeleteToken(ctx.Context)
	if err != nil {
		return fmt.Errorf("deleting token: %w", err)
	}

	return nil
}

func (c TenantsClient) getTenant(ctx *cli.Context) error {
	smClient, cleanup, err := c.ClientBuilder(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(ctx.Context) }()

	tenant, err := smClient.GetTenant(ctx.Context)
	if err != nil {
		return fmt.Errorf("getting tenant: %w", err)
	}

	return c.printTenant(ctx, tenant)
}

func (c TenantsClient) updateTenant(ctx *cli.Context) error {
	smClient, cleanup, err := c.ClientBuilder(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(ctx.Context) }()

	var in synthetic_monitoring.Tenant
	if err := readJsonArg(ctx.Args().First(), &in); err != nil {
		return err
	}

	out, err := smClient.UpdateTenant(ctx.Context, in)
	if err != nil {
		return fmt.Errorf("updating tenant: %w", err)
	}

	return c.printTenant(ctx, out)
}

func (c TenantsClient) printTenant(ctx *cli.Context, tenant *synthetic_monitoring.Tenant) error {
	jsonWriter := c.JsonWriterBuilder(ctx)
	if done, err := jsonWriter(tenant, "marshaling tenant"); err != nil || done {
		return err
	}

	w := c.TabWriterBuilder(ctx)
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
