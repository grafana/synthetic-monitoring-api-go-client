package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

func getCheckCommands() cli.Commands {
	return cli.Commands{
		&cli.Command{
			Name:  "list",
			Usage: "list Synthetic Monitoring checks",
			Action: func(c *cli.Context) error {
				smClient, cleanup, err := newClient(c)
				if err != nil {
					return err
				}
				defer func() { _ = cleanup(c.Context) }()

				checks, err := smClient.ListChecks(c.Context)
				if err != nil {
					return fmt.Errorf("listing checks: %w", err)
				}

				if done, err := outputJson(c, checks, "marshaling checks"); err != nil || done {
					return err
				}

				w := newTabWriter(os.Stdout)
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", "id", "type", "job", "target", "enabled")
				for _, check := range checks {
					fmt.Fprintf(
						w,
						"%d\t%s\t%s\t%s\t%t\n",
						check.Id,
						check.Type(),
						check.Job,
						check.Target,
						check.Enabled,
					)
				}
				if err := w.Flush(); err != nil {
					return fmt.Errorf("flushing output: %w", err)
				}

				return nil
			},
		},
	}
}
