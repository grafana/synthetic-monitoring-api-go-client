package main

import (
	"context"
	"fmt"
	"os"

	"github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/urfave/cli/v2"
)

func getProbeCommands() cli.Commands {
	return cli.Commands{
		&cli.Command{
			Name:   "list",
			Usage:  "list Synthetic Monitoring probes",
			Action: listProbes,
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
			Action: addProbe,
		},
		&cli.Command{
			Name:   "update",
			Usage:  "update a Synthetic Monitoring probe",
			Action: updateProbe,
			Flags: []cli.Flag{
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
		},
	}
}

func listProbes(c *cli.Context) error {
	smClient, cleanup, err := newClient(c)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(c.Context) }()

	probes, err := smClient.ListProbes(c.Context)
	if err != nil {
		return fmt.Errorf("listing probes: %w", err)
	}

	if done, err := outputJson(c, probes, "marshaling probes"); err != nil || done {
		return err
	}

	w := newTabWriter(os.Stdout)
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", "id", "name", "region", "public", "deprecated", "online")
	for _, p := range probes {
		fmt.Fprintf(w, "%d\t%s\t%s\t%t\t%t\t%t\n", p.Id, p.Name, p.Region, p.Public, p.Deprecated, p.Online)
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flushing output: %w", err)
	}

	return nil
}

func addProbe(c *cli.Context) error {
	smClient, cleanup, err := newClient(c)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(c.Context) }()

	newProbe, newProbeToken, err := smClient.AddProbe(c.Context, sm.Probe{
		Name:      c.String("name"),
		Latitude:  float32(c.Float64("latitude")),
		Longitude: float32(c.Float64("longitude")),
		Region:    c.String("region"),
	})
	if err != nil {
		return fmt.Errorf("adding probe: %w", err)
	}

	if c.Bool("json") {
		out := map[string]interface{}{
			"probe": newProbe,
			"token": string(newProbeToken),
		}
		if done, err := outputJson(c, out, "marshaling probe"); err != nil || done {
			return err
		}
	}

	w := newTabWriter(os.Stdout)
	fmt.Fprintf(w, "%s:\t%s\n", "name", newProbe.Name)
	fmt.Fprintf(w, "%s:\t%f\n", "latitude", newProbe.Latitude)
	fmt.Fprintf(w, "%s:\t%f\n", "longitude", newProbe.Longitude)
	fmt.Fprintf(w, "%s:\t%s\n", "region", newProbe.Region)
	fmt.Fprintf(w, "%s:\t%s\n", "token", string(newProbeToken))

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flushing output: %w", err)
	}

	return nil
}

func updateProbe(c *cli.Context) error {
	smClient, cleanup, err := newClient(c)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(c.Context) }()

	var probeUpdateFunc func(ctx context.Context, probe synthetic_monitoring.Probe) (*synthetic_monitoring.Probe, []byte, error)

	if c.Bool("reset-token") {
		probeUpdateFunc = smClient.ResetProbeToken
	} else {
		probeUpdateFunc = func(ctx context.Context, probe synthetic_monitoring.Probe) (*synthetic_monitoring.Probe, []byte, error) {
			newProbe, err := smClient.UpdateProbe(ctx, probe)

			return newProbe, nil, err //nolint:wrapcheck
		}
	}

	newProbe, newProbeToken, err := probeUpdateFunc(c.Context, sm.Probe{
		Id:        c.Int64("id"),
		Name:      c.String("name"),
		Latitude:  float32(c.Float64("latitude")),
		Longitude: float32(c.Float64("longitude")),
		Region:    c.String("region"),
	})
	if err != nil {
		return fmt.Errorf("updating probe: %w", err)
	}

	if c.Bool("json") {
		out := map[string]interface{}{
			"probe": newProbe,
			"token": string(newProbeToken),
		}
		if done, err := outputJson(c, out, "marshaling probe"); err != nil || done {
			return err
		}
	}

	w := newTabWriter(os.Stdout)
	fmt.Fprintf(w, "%s:\t%s\n", "name", newProbe.Name)
	fmt.Fprintf(w, "%s:\t%f\n", "latitude", newProbe.Latitude)
	fmt.Fprintf(w, "%s:\t%f\n", "longitude", newProbe.Longitude)
	fmt.Fprintf(w, "%s:\t%s\n", "region", newProbe.Region)
	fmt.Fprintf(w, "%s:\t%s\n", "token", string(newProbeToken))

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flushing output: %w", err)
	}

	return nil
}
