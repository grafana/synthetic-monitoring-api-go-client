package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	sm "github.com/grafana/synthetic-monitoring-agent/pkg/pb/synthetic_monitoring"
	"github.com/urfave/cli/v2"
)

func getCommonCheckFlags() []cli.Flag {
	const defaultFrequency = 60 * time.Second
	const defaultTimeout = 5 * time.Second

	return []cli.Flag{
		&cli.DurationFlag{
			Name:  "frequency",
			Usage: "frequency of the check",
			Value: defaultFrequency,
		},
		&cli.DurationFlag{
			Name:  "timeout",
			Usage: "timeout of the check",
			Value: defaultTimeout,
		},
		&cli.StringFlag{
			Name:     "job",
			Usage:    "job of the check",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "target",
			Usage:    "target of the check",
			Required: true,
		},
		&cli.BoolFlag{
			Name:  "enabled",
			Usage: "whether the check is enabled",
			Value: true,
		},
		&cli.StringSliceFlag{
			Name:  "probes",
			Usage: "names or IDs of the probes where this check should run",
			Value: cli.NewStringSlice("all"),
		},
	}
}

func getCheckCommands() cli.Commands {
	const defaultDNSPort = 53

	commands := cli.Commands{
		&cli.Command{
			Name:   "list",
			Usage:  "list Synthetic Monitoring checks",
			Action: checkList,
		},
		&cli.Command{
			Name:   "get",
			Usage:  "get a Synthetic Monitoring check",
			Action: checkGet,
			Flags: []cli.Flag{
				&cli.Int64Flag{
					Name:     "id",
					Usage:    "id of the check to get",
					Required: true,
				},
			},
		},
		&cli.Command{
			Name: "add",
			Subcommands: []*cli.Command{
				{
					Name:   "ping",
					Usage:  "add a Synthetic Monitoring ping check",
					Action: checkAddPing,
					Flags: []cli.Flag{
						&cli.GenericFlag{
							Name:  "ip-version",
							Usage: "IP version to use to connect to the target",
							Value: newIpVersion(sm.IpVersion_Any),
						},
						&cli.BoolFlag{
							Name:  "dont-fragment",
							Usage: "set the DF flag for the ICMP packet",
						},
					},
				},
				{
					Name:   "http",
					Usage:  "add a Synthetic Monitoring http check",
					Action: checkAddHttp,
					Flags: []cli.Flag{
						&cli.GenericFlag{
							Name:  "ip-version",
							Usage: "IP version to use to connect to the target",
							Value: newIpVersion(sm.IpVersion_Any),
						},
						&cli.GenericFlag{
							Name:  "method",
							Usage: "method of the request",
							Value: newHttpMethod(sm.HttpMethod_GET),
						},
						&cli.StringSliceFlag{
							Name:  "headers",
							Usage: "headers of the request",
						},
						&cli.StringFlag{
							Name:  "body",
							Usage: "body of the request",
						},
						&cli.BoolFlag{
							Name:  "no-follow-redirects",
							Usage: "do not follow redirects",
						},
						&cli.StringFlag{
							Name:  "bearer-token",
							Usage: "bearer token of the request",
						},
						&cli.BoolFlag{
							Name:  "fail-if-ssl",
							Usage: "fail if any requests goes over SSL",
						},
						&cli.BoolFlag{
							Name:  "fail-if-not-ssl",
							Usage: "fail if any requests does not go over SSL",
						},
						&cli.IntSliceFlag{
							Name:  "valid-status-codes",
							Usage: "valid HTTP status codes",
						},
						&cli.StringSliceFlag{
							Name:  "valid-http-versions",
							Usage: "valid HTTP versions",
						},
						&cli.StringSliceFlag{
							Name:  "fail-if-body-matches-regexp",
							Usage: "fail if the body matches any of the provided regular expressions",
						},
						&cli.StringSliceFlag{
							Name:  "fail-if-body-not-matches-regexp",
							Usage: "fail if the body does not match any of the provided regular expressions",
						},
						&cli.StringSliceFlag{
							Name:  "fail-if-header-matches-regexp",
							Usage: "fail if the headers match any of the provided regular expressions",
						},
						&cli.StringSliceFlag{
							Name:  "fail-if-header-not-matches-regexp",
							Usage: "fail if the headers do not match any of the provided regular expressions",
						},
						&cli.GenericFlag{
							Name:  "compression-algorithm",
							Usage: "decode responses using the specified compression algorithm",
							Value: newCompressionAlgo(sm.CompressionAlgorithm_none),
						},
						&cli.StringFlag{
							Name:  "cache-busting-parameter-name",
							Usage: "name of the query parameter to add to the request to bust the cache",
						},
					},
				},
				{
					Name:   "dns",
					Usage:  "add a Synthetic Monitoring dns check",
					Action: checkAddDns,
					Flags: []cli.Flag{
						&cli.GenericFlag{
							Name:  "ip-version",
							Usage: "IP version to use to connect to the target",
							Value: newIpVersion(sm.IpVersion_Any),
						},
						&cli.StringFlag{
							Name:  "server",
							Usage: "server to query",
						},
						&cli.IntFlag{
							Name:  "port",
							Usage: "port to query",
							Value: defaultDNSPort,
						},
						&cli.GenericFlag{
							Name:  "record-type",
							Usage: "record type to query",
							Value: newDnsRecordType(sm.DnsRecordType_A),
						},
						&cli.GenericFlag{
							Name:  "protocol",
							Usage: "protocol to use to query the server",
							Value: newDnsProtocol(sm.DnsProtocol_UDP),
						},
						&cli.StringSliceFlag{
							Name:  "valid-rcodes",
							Usage: "valid response codes",
						},
						// ValidateAnswer       *DNSRRValidator
						// ValidateAuthority    *DNSRRValidator
						// ValidateAdditional   *DNSRRValidator
					},
				},
				{
					Name:   "tcp",
					Usage:  "add a Synthetic Monitoring tcp check",
					Action: checkAddTcp,
					Flags: []cli.Flag{
						&cli.GenericFlag{
							Name:  "ip-version",
							Usage: "IP version to use to connect to the target",
							Value: newIpVersion(sm.IpVersion_Any),
						},
						// Tls                  bool               `protobuf:"varint,3,opt,name=tls,proto3" json:"tls,omitempty"`
						&cli.BoolFlag{
							Name:  "tls",
							Usage: "use TLS to connect to the target",
						},
						// TlsConfig            *TLSConfig         `protobuf:"bytes,4,opt,name=tlsConfig,proto3" json:"tlsConfig,omitempty"`
						// InsecureSkipVerify   bool     `protobuf:"varint,1,opt,name=insecureSkipVerify,proto3" json:"insecureSkipVerify,omitempty"`
						&cli.BoolFlag{
							Name:  "tls-insecure-skip-verify",
							Usage: "skip verification of the server certificate",
						},
						// CACert               []byte   `protobuf:"bytes,2,opt,name=CACert,proto3" json:"caCert,omitempty"`
						&cli.StringFlag{
							Name:  "tls-ca-cert",
							Usage: "CA certificate to use to verify the server certificate",
						},
						// ClientCert           []byte   `protobuf:"bytes,3,opt,name=clientCert,proto3" json:"clientCert,omitempty"`
						&cli.StringFlag{
							Name:  "tls-client-cert",
							Usage: "client certificate to use to connect to the target",
						},
						// ClientKey            []byte   `protobuf:"bytes,4,opt,name=clientKey,proto3" json:"clientKey,omitempty"`
						&cli.StringFlag{
							Name:  "tls-client-key",
							Usage: "client key to use to connect to the target",
						},
						// ServerName           string   `protobuf:"bytes,5,opt,name=serverName,proto3" json:"serverName,omitempty"`
						&cli.StringFlag{
							Name:  "tls-server-name",
							Usage: "server name to use to connect to the target",
						},
						// QueryResponse        []TCPQueryResponse `protobuf:"bytes,5,rep,name=queryResponse,proto3" json:"queryResponse,omitempty"`
					},
				},
			},
		},
	}

	for _, cmd := range commands {
		if cmd.Name != "add" {
			continue
		}
		for _, subCmd := range cmd.Subcommands {
			commonCheckFlags := getCommonCheckFlags()
			flags := make([]cli.Flag, 0, len(commonCheckFlags)+len(subCmd.Flags))
			flags = append(flags, commonCheckFlags...)
			flags = append(flags, subCmd.Flags...)
			subCmd.Flags = flags
		}
	}

	return commands
}

func checkList(c *cli.Context) error {
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
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", "id", "type", "job", "target", "enabled", "frequency", "timeout")
	for _, check := range checks {
		fmt.Fprintf(
			w,
			"%d\t%s\t%s\t%s\t%t\t%s\t%s\n",
			check.Id,
			check.Type(),
			check.Job,
			check.Target,
			check.Enabled,
			time.Duration(check.Frequency)*time.Millisecond,
			time.Duration(check.Timeout)*time.Millisecond,
		)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flushing output: %w", err)
	}

	return nil
}

func checkGet(c *cli.Context) error {
	smClient, cleanup, err := newClient(c)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(c.Context) }()

	check, err := smClient.GetCheck(c.Context, c.Int64("id"))
	if err != nil {
		return fmt.Errorf("getting check: %w", err)
	}

	if done, err := outputJson(c, check, "marshaling check"); err != nil || done {
		return err
	}

	return showCheck(os.Stdout, check)
}

func checkAddPing(c *cli.Context) error {
	smClient, cleanup, err := newClient(c)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(c.Context) }()

	ipVersion := sm.IpVersion(*(c.Generic("ip-version").(*ipVersion)))

	probes, err := smClient.ListProbes(c.Context)
	if err != nil {
		return fmt.Errorf("getting probes: %w", err)
	}

	check := sm.Check{
		Job:       c.String("job"),
		Target:    c.String("target"),
		Frequency: c.Duration("frequency").Milliseconds(),
		Timeout:   c.Duration("timeout").Milliseconds(),
		Enabled:   c.Bool("enabled"),
		Settings: sm.CheckSettings{
			Ping: &sm.PingSettings{
				IpVersion:    ipVersion,
				DontFragment: c.Bool("dont-fragment"),
			},
		},
	}

	wantedProbes := make(map[string]struct{})

	for _, probe := range c.StringSlice("probes") {
		wantedProbes[strings.ToLower(strings.TrimSpace(probe))] = struct{}{}
	}

	if _, found := wantedProbes["all"]; found {
		for _, probe := range probes {
			check.Probes = append(check.Probes, probe.Id)
		}
	} else {
		for _, probe := range probes {
			if _, found := wantedProbes[strings.ToLower(probe.Name)]; found {
				check.Probes = append(check.Probes, probe.Id)
			} else if _, found := wantedProbes[idToStr(probe.Id)]; found {
				check.Probes = append(check.Probes, probe.Id)
			}
		}
	}

	if err := check.Validate(); err != nil {
		return fmt.Errorf("invalid check: %w", err)
	}

	newCheck, err := smClient.AddCheck(c.Context, check)
	if err != nil {
		return fmt.Errorf("adding check: %w", err)
	}

	if done, err := outputJson(c, newCheck, "marshaling check"); err != nil || done {
		return err
	}

	return showCheck(os.Stdout, newCheck)
}

func checkAddHttp(c *cli.Context) error {
	smClient, cleanup, err := newClient(c)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(c.Context) }()

	ipVersion := sm.IpVersion(*(c.Generic("ip-version").(*ipVersion)))
	httpMethod := sm.HttpMethod(*(c.Generic("method").(*httpMethod)))
	compressionAlgo := sm.CompressionAlgorithm(*(c.Generic("compression-algorithm").(*compressionAlgo)))

	var validHttpStatusCodes []int32
	if c.IsSet("http-status-codes") {
		in := c.IntSlice("http-status-codes")
		validHttpStatusCodes = make([]int32, 0, len(in))
		for _, statusCode := range in {
			validHttpStatusCodes = append(validHttpStatusCodes, int32(statusCode))
		}
	}

	probes, err := smClient.ListProbes(c.Context)
	if err != nil {
		return fmt.Errorf("getting probes: %w", err)
	}

	check := sm.Check{
		Job:       c.String("job"),
		Target:    c.String("target"),
		Frequency: c.Duration("frequency").Milliseconds(),
		Timeout:   c.Duration("timeout").Milliseconds(),
		Enabled:   c.Bool("enabled"),
		Settings: sm.CheckSettings{
			Http: &sm.HttpSettings{
				IpVersion:                  ipVersion,
				Method:                     httpMethod,
				Headers:                    c.StringSlice("headers"),
				Body:                       c.String("body"),
				NoFollowRedirects:          c.Bool("no-follow-redirects"),
				BearerToken:                c.String("bearer-token"),
				FailIfSSL:                  c.Bool("fail-if-ssl"),
				FailIfNotSSL:               c.Bool("fail-if-not-ssl"),
				ValidStatusCodes:           validHttpStatusCodes,
				ValidHTTPVersions:          c.StringSlice("valid-http-versions"),
				FailIfBodyMatchesRegexp:    c.StringSlice("fail-if-body-matches-regexp"),
				FailIfBodyNotMatchesRegexp: c.StringSlice("fail-if-body-not-matches-regexp"),
				// FailIfHeaderMatchesRegexp:    c.StringSlice("fail-if-header-matches-regexp"),
				// FailIfHeaderNotMatchesRegexp: c.StringSlice("fail-if-header-not-matches-regexp"),
				Compression:                compressionAlgo,
				CacheBustingQueryParamName: c.String("cache-busting-query-param-name"),
			},
		},
	}

	wantedProbes := make(map[string]struct{})

	for _, probe := range c.StringSlice("probes") {
		wantedProbes[strings.ToLower(strings.TrimSpace(probe))] = struct{}{}
	}

	if _, found := wantedProbes["all"]; found {
		for _, probe := range probes {
			check.Probes = append(check.Probes, probe.Id)
		}
	} else {
		for _, probe := range probes {
			if _, found := wantedProbes[strings.ToLower(probe.Name)]; found {
				check.Probes = append(check.Probes, probe.Id)
			} else if _, found := wantedProbes[idToStr(probe.Id)]; found {
				check.Probes = append(check.Probes, probe.Id)
			}
		}
	}

	if err := check.Validate(); err != nil {
		return fmt.Errorf("invalid check: %w", err)
	}

	newCheck, err := smClient.AddCheck(c.Context, check)
	if err != nil {
		return fmt.Errorf("adding check: %w", err)
	}

	if done, err := outputJson(c, newCheck, "marshaling check"); err != nil || done {
		return err
	}

	return showCheck(os.Stdout, newCheck)
}

func checkAddDns(c *cli.Context) error {
	smClient, cleanup, err := newClient(c)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(c.Context) }()

	ipVersion := sm.IpVersion(*(c.Generic("ip-version").(*ipVersion)))

	check := sm.Check{
		Job:       c.String("job"),
		Target:    c.String("target"),
		Frequency: c.Duration("frequency").Milliseconds(),
		Timeout:   c.Duration("timeout").Milliseconds(),
		Enabled:   c.Bool("enabled"),
		Settings: sm.CheckSettings{
			Dns: &sm.DnsSettings{
				IpVersion:   ipVersion,
				Server:      c.String("server"),
				Port:        int32(c.Int("port")),
				RecordType:  sm.DnsRecordType(*(c.Generic("record-type").(*dnsRecordType))),
				Protocol:    sm.DnsProtocol(*(c.Generic("protocol").(*dnsProtocol))),
				ValidRCodes: c.StringSlice("valid-rcodes"),
				// ValidateAnswer       *DNSRRValidator
				// ValidateAuthority    *DNSRRValidator
				// ValidateAdditional   *DNSRRValidator
			},
		},
	}

	probes, err := smClient.ListProbes(c.Context)
	if err != nil {
		return fmt.Errorf("getting probes: %w", err)
	}

	wantedProbes := make(map[string]struct{})

	for _, probe := range c.StringSlice("probes") {
		wantedProbes[strings.ToLower(strings.TrimSpace(probe))] = struct{}{}
	}

	if _, found := wantedProbes["all"]; found {
		for _, probe := range probes {
			check.Probes = append(check.Probes, probe.Id)
		}
	} else {
		for _, probe := range probes {
			if _, found := wantedProbes[strings.ToLower(probe.Name)]; found {
				check.Probes = append(check.Probes, probe.Id)
			} else if _, found := wantedProbes[idToStr(probe.Id)]; found {
				check.Probes = append(check.Probes, probe.Id)
			}
		}
	}

	if err := check.Validate(); err != nil {
		return fmt.Errorf("invalid check: %w", err)
	}

	newCheck, err := smClient.AddCheck(c.Context, check)
	if err != nil {
		return fmt.Errorf("adding check: %w", err)
	}

	if done, err := outputJson(c, newCheck, "marshaling check"); err != nil || done {
		return err
	}

	return showCheck(os.Stdout, newCheck)
}

func checkAddTcp(c *cli.Context) error {
	smClient, cleanup, err := newClient(c)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup(c.Context) }()

	ipVersion := sm.IpVersion(*(c.Generic("ip-version").(*ipVersion)))

	check := sm.Check{
		Job:       c.String("job"),
		Target:    c.String("target"),
		Frequency: c.Duration("frequency").Milliseconds(),
		Timeout:   c.Duration("timeout").Milliseconds(),
		Enabled:   c.Bool("enabled"),
		Settings: sm.CheckSettings{
			Tcp: &sm.TcpSettings{
				IpVersion: ipVersion,
				Tls:       c.Bool("tls"),
				TlsConfig: &sm.TLSConfig{
					InsecureSkipVerify: c.Bool("tls-insecure-skip-verify"),
					CACert:             []byte(c.String("tls-ca-cert")),
					ClientCert:         []byte(c.String("tls-client-cert")),
					ClientKey:          []byte(c.String("tls-client-key")),
					ServerName:         c.String("tls-server-name"),
				},
				// QueryResponse        []TCPQueryResponse `protobuf:"bytes,5,rep,name=queryResponse,proto3" json:"queryResponse,omitempty"`
			},
		},
	}

	probes, err := smClient.ListProbes(c.Context)
	if err != nil {
		return fmt.Errorf("getting probes: %w", err)
	}

	wantedProbes := make(map[string]struct{})

	for _, probe := range c.StringSlice("probes") {
		wantedProbes[strings.ToLower(strings.TrimSpace(probe))] = struct{}{}
	}

	if _, found := wantedProbes["all"]; found {
		for _, probe := range probes {
			check.Probes = append(check.Probes, probe.Id)
		}
	} else {
		for _, probe := range probes {
			if _, found := wantedProbes[strings.ToLower(probe.Name)]; found {
				check.Probes = append(check.Probes, probe.Id)
			} else if _, found := wantedProbes[idToStr(probe.Id)]; found {
				check.Probes = append(check.Probes, probe.Id)
			}
		}
	}

	if err := check.Validate(); err != nil {
		return fmt.Errorf("invalid check: %w", err)
	}

	newCheck, err := smClient.AddCheck(c.Context, check)
	if err != nil {
		return fmt.Errorf("adding check: %w", err)
	}

	if done, err := outputJson(c, newCheck, "marshaling check"); err != nil || done {
		return err
	}

	return showCheck(os.Stdout, newCheck)
}

func showCheck(output io.Writer, check *sm.Check) error {
	w := newTabWriter(output)
	fmt.Fprintf(w, "%s:\t%d\n", "id", check.Id)
	fmt.Fprintf(w, "%s:\t%s\n", "type", check.Type())
	fmt.Fprintf(w, "%s:\t%s\n", "job", check.Job)
	fmt.Fprintf(w, "%s:\t%s\n", "target", check.Target)
	fmt.Fprintf(w, "%s:\t%t\n", "enabled", check.Enabled)
	fmt.Fprintf(w, "%s:\t%s\n", "frequency", time.Duration(check.Frequency)*time.Millisecond)
	fmt.Fprintf(w, "%s:\t%s\n", "timeout", time.Duration(check.Timeout)*time.Millisecond)
	fmt.Fprintf(w, "%s:\t%s\n", "created", formatSMTime(check.Created))
	fmt.Fprintf(w, "%s:\t%s\n", "modified", formatSMTime(check.Modified))

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flushing output: %w", err)
	}

	return nil
}

func valueToString(value interface{}) string {
	buf, err := json.Marshal(value)
	if err != nil {
		return "<invalid>"
	}

	out, err := strconv.Unquote(string(buf))
	if err != nil {
		return "<invalid>"
	}

	return out
}

type ipVersion sm.IpVersion

func (v *ipVersion) Set(value string) error {
	var tmp sm.IpVersion

	if err := json.Unmarshal([]byte(`"`+value+`"`), &tmp); err != nil {
		return fmt.Errorf("parsing ip version: %w", err)
	}

	*v = ipVersion(tmp)

	return nil
}

func (v *ipVersion) String() string {
	tmp := sm.IpVersion(*v)

	return valueToString(&tmp)
}

func newIpVersion(v sm.IpVersion) *ipVersion {
	tmp := ipVersion(v)

	return &tmp
}

type httpMethod sm.HttpMethod

func (v *httpMethod) Set(value string) error {
	var tmp sm.HttpMethod

	if err := json.Unmarshal([]byte(`"`+value+`"`), &tmp); err != nil {
		return fmt.Errorf("parsing http method: %w", err)
	}

	*v = httpMethod(tmp)

	return nil
}

func (v *httpMethod) String() string {
	tmp := sm.HttpMethod(*v)

	return valueToString(&tmp)
}

func newHttpMethod(v sm.HttpMethod) *httpMethod {
	tmp := httpMethod(v)

	return &tmp
}

type compressionAlgo sm.CompressionAlgorithm

func (v *compressionAlgo) Set(value string) error {
	var tmp sm.CompressionAlgorithm

	if err := json.Unmarshal([]byte(`"`+value+`"`), &tmp); err != nil {
		return fmt.Errorf("parsing compression algorithm: %w", err)
	}

	*v = compressionAlgo(tmp)

	return nil
}

func (v *compressionAlgo) String() string {
	tmp := sm.CompressionAlgorithm(*v)

	return valueToString(&tmp)
}

func newCompressionAlgo(v sm.CompressionAlgorithm) *compressionAlgo {
	tmp := compressionAlgo(v)

	return &tmp
}

type dnsRecordType sm.DnsRecordType

func (v *dnsRecordType) Set(value string) error {
	var tmp sm.DnsRecordType

	if err := json.Unmarshal([]byte(`"`+value+`"`), &tmp); err != nil {
		return fmt.Errorf("parsing dns record type: %w", err)
	}

	*v = dnsRecordType(tmp)

	return nil
}

func (v *dnsRecordType) String() string {
	tmp := sm.DnsRecordType(*v)

	return valueToString(&tmp)
}

func newDnsRecordType(v sm.DnsRecordType) *dnsRecordType {
	tmp := dnsRecordType(v)

	return &tmp
}

type dnsProtocol sm.DnsProtocol

func (v *dnsProtocol) Set(value string) error {
	var tmp sm.DnsProtocol

	if err := json.Unmarshal([]byte(`"`+value+`"`), &tmp); err != nil {
		return fmt.Errorf("parsing dns protocol: %w", err)
	}

	*v = dnsProtocol(tmp)

	return nil
}

func (v *dnsProtocol) String() string {
	tmp := sm.DnsProtocol(*v)

	return valueToString(&tmp)
}

func newDnsProtocol(v sm.DnsProtocol) *dnsProtocol {
	tmp := dnsProtocol(v)

	return &tmp
}

func idToStr(id int64) string {
	const base = 10

	return strconv.FormatInt(id, base)
}
