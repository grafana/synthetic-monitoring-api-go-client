package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	smapi "github.com/grafana/synthetic-monitoring-api-go-client"
	"github.com/urfave/cli/v2"
)

type WriteFlusher interface {
	io.Writer
	Flush() error
}

type ServiceClient struct {
	ClientBuilder     func(*cli.Context) (*smapi.Client, func(context.Context) error, error)
	JsonWriterBuilder func(*cli.Context) func(interface{}, string) (bool, error)
	TabWriterBuilder  func(*cli.Context) WriteFlusher
}

func formatSMTime(t float64) string {
	return time.Unix(int64(t), 0).Format(time.RFC3339)
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
