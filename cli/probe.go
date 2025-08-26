package cli

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/urfave/cli/v2"
)

var errInvalidLabel = errors.New("invalid label")

func GetProbeCommands(c ProbesClient) cli.Commands {
	return cli.Commands{
		&cli.Command{
			Name:   "list",
			Usage:  "list Synthetic Monitoring probes",
			Action: c.listProbes,
		},
		&cli.Command{
			Name: "add",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "name",
					Usage: "name of the probe",
				},
				&cli.Float64Flag{
					Name:    "latitude",
					Aliases: []string{"lat"},
					Usage:   "latitude of the probe",
				},
				&cli.Float64Flag{
					Name:    "longitude",
					Aliases: []string{"long"},
					Usage:   "longitude of the probe",
				},
				&cli.StringFlag{
					Name:  "region",
					Usage: "region of the probe",
				},
			},
			Usage:  "add a Synthetic Monitoring probe",
			Action: c.addProbe,
		},
		&cli.Command{
			Name:   "get",
			Usage:  "get a Synthetic Monitoring probe",
			Action: c.getProbe,
			Flags: []cli.Flag{
				&cli.Int64Flag{
					Name:     "id",
					Usage:    "id of the probe to get",
					Required: true,
				},
			},
		},
		&cli.Command{
			Name:   "update",
			Usage:  "update a Synthetic Monitoring probe",
			Action: c.updateProbe,
			Flags: []cli.Flag{
				&cli.Int64Flag{
					Name:     "id",
					Usage:    "id of the probe to update",
					Required: true,
				},
				&cli.Float64Flag{
					Name:    "latitude",
					Aliases: []string{"lat"},
					Usage:   "new latitude of the probe",
				},
				&cli.Float64Flag{
					Name:    "longitude",
					Aliases: []string{"long"},
					Usage:   "new longitude of the probe",
				},
				&cli.StringFlag{
					Name:  "region",
					Usage: "new region of the probe",
				},
				&cli.BoolFlag{
					Name:  "deprecated",
					Usage: "whether the probe is deprecated",
				},
				&cli.StringSliceFlag{
					Name:  "labels",
					Usage: "new labels for the probe",
				},
				&cli.BoolFlag{
					Name:  "reset-token",
					Usage: "reset the probe's access token",
				},
			},
		},
		&cli.Command{
			Name:   "delete",
			Usage:  "delete one or more Synthetic Monitoring probes",
			Action: c.deleteProbe,
			Flags: []cli.Flag{
				&cli.Int64SliceFlag{
					Name:     "id",
					Usage:    "id of the probe to delete",
					Required: true,
				},
			},
		},
	}
}

type ProbesClient ServiceClient

func (c ProbesClient) listProbes(ctx *cli.Context) error {
	smClient, cleanup, err := c.ClientBuilder(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(ctx.Context) }()

	probes, err := smClient.ListProbes(ctx.Context)
	if err != nil {
		return fmt.Errorf("listing probes: %w", err)
	}

	jsonWriter := c.JsonWriterBuilder(ctx)
	if done, err := jsonWriter(probes, "marshaling probes"); err != nil || done {
		return err
	}

	w := c.TabWriterBuilder(ctx)
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", "id", "name", "region", "latitude", "longitude", "public", "deprecated", "online")
	for _, p := range probes {
		fmt.Fprintf(w, "%d\t%s\t%s\t%.3f\t%.3f\t%t\t%t\t%t\n", p.Id, p.Name, p.Region, p.Latitude, p.Longitude, p.Public, p.Deprecated, p.Online)
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flushing output: %w", err)
	}

	return nil
}

func (c ProbesClient) addProbe(ctx *cli.Context) error {
	smClient, cleanup, err := c.ClientBuilder(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(ctx.Context) }()

	newProbe, newProbeToken, err := smClient.AddProbe(ctx.Context, sm.Probe{
		Name:      ctx.String("name"),
		Latitude:  float32(ctx.Float64("latitude")),
		Longitude: float32(ctx.Float64("longitude")),
		Region:    ctx.String("region"),
	})
	if err != nil {
		return fmt.Errorf("adding probe: %w", err)
	}

	if ctx.Bool("json") {
		out := map[string]interface{}{
			"probe": newProbe,
			"token": string(newProbeToken),
		}
		jsonWriter := c.JsonWriterBuilder(ctx)
		if done, err := jsonWriter(out, "marshaling probe"); err != nil || done {
			return err
		}
	}

	w := c.TabWriterBuilder(ctx)
	fmt.Fprintf(w, "%s:\t%s\n", "name", newProbe.Name)
	fmt.Fprintf(w, "%s:\t%s\n", "region", newProbe.Region)
	fmt.Fprintf(w, "%s:\t%f\n", "latitude", newProbe.Latitude)
	fmt.Fprintf(w, "%s:\t%f\n", "longitude", newProbe.Longitude)
	fmt.Fprintf(w, "%s:\t%t\n", "deprecated", newProbe.Deprecated)
	fmt.Fprintf(w, "%s:\t%t\n", "public", newProbe.Public)
	fmt.Fprintf(w, "%s:\t%s\n", "created", time.Unix(int64(newProbe.Created), 0))
	fmt.Fprintf(w, "%s:\t%s\n", "modified", time.Unix(int64(newProbe.Modified), 0))
	fmt.Fprintf(w, "%s:\t%s\n", "token", string(newProbeToken))

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flushing output: %w", err)
	}

	return nil
}

func (c ProbesClient) getProbe(ctx *cli.Context) error {
	smClient, cleanup, err := c.ClientBuilder(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(ctx.Context) }()

	probe, err := smClient.GetProbe(ctx.Context, ctx.Int64("id"))
	if err != nil {
		return fmt.Errorf("getting probe: %w", err)
	}

	jsonWriter := c.JsonWriterBuilder(ctx)
	if done, err := jsonWriter(probe, "marshaling probe"); err != nil || done {
		return err
	}

	w := c.TabWriterBuilder(ctx)
	fmt.Fprintf(w, "%s:\t%s\n", "name", probe.Name)
	fmt.Fprintf(w, "%s:\t%s\n", "region", probe.Region)
	fmt.Fprintf(w, "%s:\t%f\n", "latitude", probe.Latitude)
	fmt.Fprintf(w, "%s:\t%f\n", "longitude", probe.Longitude)
	fmt.Fprintf(w, "%s:\t%t\n", "deprecated", probe.Deprecated)
	fmt.Fprintf(w, "%s:\t%t\n", "public", probe.Public)
	fmt.Fprintf(w, "%s:\t%s\n", "created", formatSMTime(probe.Created))
	fmt.Fprintf(w, "%s:\t%s\n", "modified", formatSMTime(probe.Modified))

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flushing output: %w", err)
	}

	return nil
}

func (c ProbesClient) updateProbe(ctx *cli.Context) error {
	smClient, cleanup, err := c.ClientBuilder(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(ctx.Context) }()

	var probeUpdateFunc func(ctx context.Context, probe sm.Probe) (*sm.Probe, []byte, error)

	if ctx.Bool("reset-token") {
		probeUpdateFunc = smClient.ResetProbeToken
	} else {
		probeUpdateFunc = func(ctx context.Context, probe sm.Probe) (*sm.Probe, []byte, error) {
			newProbe, err := smClient.UpdateProbe(ctx, probe)

			return newProbe, nil, err //nolint:wrapcheck // this function is an adapter
		}
	}

	probe, err := smClient.GetProbe(ctx.Context, ctx.Int64("id"))
	if err != nil {
		return fmt.Errorf("getting probe: %w", err)
	}

	if ctx.IsSet("latitude") {
		probe.Latitude = float32(ctx.Float64("latitude"))
	}

	if ctx.IsSet("longitude") {
		probe.Longitude = float32(ctx.Float64("longitude"))
	}

	if ctx.IsSet("region") {
		probe.Region = ctx.String("region")
	}

	if ctx.IsSet("deprecated") {
		probe.Deprecated = ctx.Bool("deprecated")
	}

	if ctx.IsSet("labels") {
		labels := ctx.StringSlice("labels")
		probe.Labels = make([]sm.Label, 0, len(labels))

		for _, label := range labels {
			const labelParts = 2
			parts := strings.SplitN(label, "=", labelParts)
			if len(parts) != labelParts {
				return fmt.Errorf("%q: %w", label, errInvalidLabel)
			}
			probe.Labels = append(probe.Labels, sm.Label{
				Name:  parts[0],
				Value: parts[1],
			})
		}
	}

	newProbe, newProbeToken, err := probeUpdateFunc(ctx.Context, *probe)
	if err != nil {
		return fmt.Errorf("updating probe: %w", err)
	}

	var token string
	if len(newProbeToken) > 0 {
		token = base64.StdEncoding.EncodeToString(newProbeToken)
	}

	if ctx.Bool("json") {
		out := map[string]interface{}{
			"probe": newProbe,
			"token": token,
		}
		jsonWriter := c.JsonWriterBuilder(ctx)
		if done, err := jsonWriter(out, "marshaling probe"); err != nil || done {
			return err
		}
	}

	w := c.TabWriterBuilder(ctx)
	fmt.Fprintf(w, "%s:\t%s\n", "name", newProbe.Name)
	fmt.Fprintf(w, "%s:\t%s\n", "region", newProbe.Region)
	fmt.Fprintf(w, "%s:\t%f\n", "latitude", newProbe.Latitude)
	fmt.Fprintf(w, "%s:\t%f\n", "longitude", newProbe.Longitude)
	fmt.Fprintf(w, "%s:\t%t\n", "deprecated", newProbe.Deprecated)
	fmt.Fprintf(w, "%s:\t%t\n", "public", newProbe.Public)
	fmt.Fprintf(w, "%s:\t%s\n", "created", time.Unix(int64(newProbe.Created), 0))
	fmt.Fprintf(w, "%s:\t%s\n", "modified", time.Unix(int64(newProbe.Modified), 0))
	if len(newProbeToken) > 0 {
		fmt.Fprintf(w, "%s:\t%s\n", "token", token)
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flushing output: %w", err)
	}

	return nil
}

func (c ProbesClient) deleteProbe(ctx *cli.Context) error {
	smClient, cleanup, err := c.ClientBuilder(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(ctx.Context) }()

	for _, id := range ctx.Int64Slice("id") {
		err := smClient.DeleteProbe(ctx.Context, id)
		if err != nil {
			return fmt.Errorf("deleting probe %d: %w", id, err)
		}
	}

	jsonWriter := c.JsonWriterBuilder(ctx)
	if done, err := jsonWriter(struct{}{}, "marshaling result"); err != nil || done {
		return err
	}

	return nil
}
