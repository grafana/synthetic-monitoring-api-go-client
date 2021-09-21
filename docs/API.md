Synthetic Monitoring API
========================

This document describes the Synthetic Monitoring API. All the entry
points return results formatted as JSON objects.

## API URL

Based on region of your Grafana Cloud stack, you need to use a diffrent API URL

**United States Region**

Synthetic Monitoring API is hosted at [https://synthetic-monitoring-api.grafana.net](https://synthetic-monitoring-api.grafana.net)

**Europe Region**

Synthetic Monitoring API is hosted at [https://synthetic-monitoring-api-eu-west.grafana.net](https://synthetic-monitoring-api-eu-west.grafana.net)


If you are unsure about region of your stack, check **Synthetic Monitoring -> Config** page. 
API URL for your Grafana Cloud instance will be listed under **Backend address** 

> NOTE: You need to be `Admin` in to access Synthetic Monitoring config page


## Authentication

All the entry points that indicate that they require authentication must
provide an access token obtained by calling `/api/v1/register/install`.

The access token MUST be used in authenticated API calls in a
`Authorization` header like the following:

```
Authorization: Bearer <accessToken>
```

## Common types

The following types are shared among several entry points.

### hostedInstance

A `hostedInstance` represents the information about either a hosted
metric or a hosted log instance. Only hosted metric instances have
`currentActiveSeries` and `currentDpm` information.

For hosted metrics instances, the type is `prometheus`.

For hosted logs instances, the type is `logs`.

```
<hostedInstance>: {
    "id": <integer>,
    "orgSlug": <string>,
    "orgName": <string>,
    "clusterSlug": <string>,
    "clusterName": <string>,
    "type": <string>,
    "name": <string>,
    "url": <string>,
    "description": <string>,
    "status": <string>,
    "currentActiveSeries": <float>,
    "currentDpm": <float>,
    "currentUsage": <float>
}
```

Whenever a request returns an error, the response has the following format:

```
{
    "code": <int>,
    "msg": <string>,
    "err": <string>
}
```

The `msg` field is intended to be presented to the user, if necessary.

## Registration API

This part of the API is used during the setup phase of the application.

### /api/v1/register/install

Method: POST

Authorization required: yes (see description)

Content-type: application/json; charset=utf-8

Body:
```
{
	"stackId": 123,
	"metricsInstanceId": 456,
	"logsInstanceId": 789
}
```

Header:
```
Authorization: Bearer <grafana publisher token>
```

Response:
```
{
    "accessToken": <string>,
    "tenantInfo": {
        "id": <integer>,
        "metricInstance": <hostedInstance>,
        "logInstance": <hostedInstance>,
    }
}
```

Description:

This entry point sets up a new tenant for the specified stack, using the
corresponding hosted metric and log instances.

The authentication is different from all the other authenticated entry
points in that the token _is not_ the access token returned by this call
(or `/api/v1/register/init`), but instead it's a `grafana.com` API
*publisher token*. This token is user to authenticate the request and
obtain the `grafana.com` organization associated with the new tenant. It
is also saved by the Synthetic Monitoring backend and passed to the
probes so that they can publish metrics and logs to the specified hosted
instances.

A new Synthetic Monitoring tenant is created if there isn't one already
for the specified stack, or updated with the new instance details
otherwise. In either case the information for the tenant is returned in
the `tenantInfo` field. The details for hosted metrics and hosted logs
instances for this tenant are returned in the respective fields of
`tenantInfo`.

The value of the `accessToken` field MUST be used for authentication as
explained in the "authentication" section of this document.

### /api/v1/register/init

> NOTE: this endpoint will be deprecated, please use `/api/v1/register/install`

Method: POST

Authorization required: no (see description)

Content-type: application/json; charset=utf-8

Body:
```
{
    "apiToken": "grafana.com admin token"
}
```

Response:
```
{
    "accessToken": <string>,
    "tenantInfo": {
        "id": <integer>,
        "metricInstance": <hostedInstance>,
        "logInstance": <hostedInstance>,
    "instances": [
        <hostedInstance>,
        ...
    ]
}
```

Description:

The `apiToken` field provided in the body of this request MUST be an
*admin* API token obtained from [https://grafana.com/orgs/bprd/api-keys](grafana.com).
This is used to identify the organization from which hosted metrics and hosted logs
instances are obtained. This token is *not* stored, and it's the
responsibility of the caller of this entry point to remove it once it's
done.

This entry point searches for a grafana.com organization, retrieves all
the hosted metrics and hosted logs instances (returned in the
`instances` field) and creates a synthetic monitoring tenant if there
isn't one already. In either case the information for the tenant is
returned in the`tenantInfo` field. If there are hosted metrics and
hosted logs instances already selected for this tenant, those are
returned in the respective fields of `tenantInfo`. Usually the first
time this entry point is used, there is no existing tenant and there are
no hosted metrics and logs instances selected, so those fields are
absent.

The value of the `accessToken` field MUST be used for authentication as
explained in the "authentication" section of this document.

### /api/v1/register/save

> NOTE: this endpoint will be deprecated, please use `/api/v1/register/install`

Method: POST

Authorization required: yes

Content-type: application/json; charset=utf-8

Body:
```
{
    "apiToken": "grafana.com admin token",
    "metricsInstanceId": <int>,
    "logsInstanceId": <int>
}
```

Response:
```
{}
```

Description:

After calling `init`, this entry point selects the hosted metrics and
hosted logs instances that should be used by Synthetic Monitoring.

Note that _both_ authorization _and_ an admin token are required.

The instance IDs correspond to those returned by `init`.

### /api/v1/register/viewer-token

> NOTE: this endpoint will be deprecated, please use `/api/v1/register/install`

Method: POST

Authorization required: no (see description)

Content-type: application/json; charset=utf-8

Body:
```
{
    "apiToken": "grafana.com admin token",
    "id": <int>,
    "type": <string>
}
```

Response:
```
{
    "token": <string>
}
```

Description:

This entry point is used to create viewer tokens for the corresponding
instances. These viewer tokens are used by Grafana to access the
selected metric and log instances.

* `id` corresponds to the hosted instance ID for which a viewer token is
  to be created.
* `type` specifies the type of instance (`prometheus` or `logs`) and
  it's used to disambiguate the `id`.

The response contains the newly created token.

### /api/v1/register/describe

> NOTE: this endpoint will be deprecated, please use `/api/v1/register/install`

Method: POST

Authorization required: no (see description)

Content-type: application/json; charset=utf-8

Body:
```
{
    "apiToken": "grafana.com admin token"
}
```

Response:
```
{
  "orgID": <int>,
  "tenants": [
    {
      "id": <integer>,
      "metricInstance": <hostedInstance>,
      "logInstance": <hostedInstance>
    },
    ...
  ]
}
```

Description:

This entry point returns the list of tenants for the organization to
which the provided admin token belongs to.

## Tokens

The access tokens obtained using `/api/v1/register/install` can be
managed using the entrypoints described in this section.

### /api/v1/token/create

Method: POST

Authorization required: yes

Body: none

Response:
```
{
    "msg": <user facing message>,
    "accessToken": <new token>
}
```

Description:

A new access token is created for the authenticated tenant.

### /api/v1/token/delete

Method: DELETE

Authorization required: yes

Body: none

Response:
```
{
    "msg": <user facing message>,
}
```

Description:

The access token used for authentication is deleted.

### /api/v1/token/refresh

Method: POST

Authorization required: yes

Body: none

Response:
```
{
    "msg": <user facing message>,
    "accessToken": <new token>
}
```

Description:

A new access token is created for the authenticated tenant.  The token
used for authentication is deleted.

### /api/v1/token/validate

Method: POST

Authorization required: yes

Body: none

Response:
```
{
    "msg": <user facing message>,
    "isValid": true
}
```

Description:

This authenticated endpoint can be used to verify the validity of
existing tokens.

Since the call is authenticated, if the provided token is invalid the
server will return a 401 error so the "isValid" field will never be set
to false.

## Checks

### /api/v1/check/add

Method: POST

Authorization required: yes

Content-type: application/json; charset=utf-8

Body:
```
{
    "target": <string>,
    "job": <string>,
    "frequency": <int>,
    "timeout": <int>,
    "enabled": <boolean>,
    "alertSensitivity": <string>,
    "basicMetricsOnly": <boolean>,
    "probes": [
      <int>,
      ...
    ],
    "labels": [
      {
        "name": <string>,
        "value": <string>
      },
      ...
    ],
    "settings": <CheckSettings>
}
```

Check settings are always specified using the following structure:

```
<CheckSettings>: {
  "dns": <DnsSettings>,
  "http": <HttpSettings>,
  "ping": <PingSettings>,
  "tcp": <TcpSettings>
}
```

Exactly one of the fields MUST be specified.

For DNS, the structure is as follows:

```
<DnsSettings>: {
  "ipVersion": <IpVersion>,
  "sourceIpAddress": <string>,
  "server": <string>,
  "port": <int>,
  "recordType": ("ANY"|"A"|"AAAA"|"CNAME"|"MX"|"NS"|"PTR"|"SOA"|"SRV"|"TXT"),
  "protocol": ("TCP"|"UDP"),
  "validRCode": [<string>, ...],
  "validateAnswerRRS": <DnsRRValidator>,
  "validateAuthorityRRS": <DnsRRValidator>,
  "validateAditionalRRS": <DnsRRValidator>
}
```

For HTTP, the structure is as follows:

```
<HttpSettings>: {
  "ipVersion": <IpVersion>,
  "method": ("GET"|"CONNECT"|"DELETE"|"HEAD"|"OPTIONS"|"POST"|"PUT"|"TRACE"),
  "headers": [
    <string>,
    ...
  ],
  "body": [<string>],
  "noFollowRedirects": <boolean>,
  "tlsConfig": <TLSConfig>,
  "basicAuth": {
    "username": <string>,
    "password": <string>
  },
  "bearerToken": <string>,
  "proxyURL": <string>,
  "failIfSSL": <boolean>,
  "failIfNotSSL": <boolean>,
  "validStatusCodes": [
    <int>,
    ...
  ],
  "validHTTPVersions": [
    <string>,
    ...
  ],
  "failIfBodyMatchesRegexp": [
    <string>,
    ...
  ],
  "failIfBodyNotMatchesRegexp": [
    <string>,
    ...
  ],
  "failIfHeaderMatchesRegexp": [
    <HeaderMatch>,
    ...
  ],
  "failIfHeaderNotMatchesRegexp": [
    <HeaderMatch>,
    ...
  ],
  "cacheBustingQueryParamName": <string>
}
```

For ping (ICMP), the structure is as follows:

```
<PingSettings>: {
  "ipVersion": <IpVersion>,
  "sourceIpAddress": <string>,
  "payloadSize": <int>,
  "dontFragment": <boolean>
}
```

For TCP, the structure is as follows:

```
<TcpSettings>: {
  "ipVersion": <IpVersion>,
  "sourceIpAddress": <string>,
  "tls": <boolean>,
  "tlsConfig": <TLSConfig>,
  "queryResponse": [
    {
      "send": <base64-encoded data>,
      "expect": <base64-encoded data>,
      "startTLS": <boolean>
    },
    ...
  ]
}
```

The following structures are used in multiple fields:

```
<IpVersion>: ("V4"|"V6"|"Any")

<DnsRRValidator>: {
  "failIfMatchesRegexp": [<regexp>, ...],
  "failIfNotMatchesRegexp": [<regexp>, ...]
}

<TLSConfig>: {
  "insecureSkipVerify": <boolean>,
  "caCert": <base64-encoded data>,
  "clientCert": <base64-encoded data>,
  "clientKey": <base64-encoded data>,
  "serverName": <string>
}

<HeaderMatch>: {
  "header": <string>,
  "regexp": <regexp>,
  "allowMissing": <boolean>
}
```

Response:
```
{
    "id": <int>,
    "tenantId": <int>,
    "target": <string>,
    "job": <string>,
    "frequency": <int>,
    "timeout": <int>,
    "enabled": <boolean>,
    "alertSensitivity": <string>,
    "basicMetricsOnly": <boolean>,
    "probes": [
      <int>,
      ...
    ],
    "labels": [
      {
        "name": <string>,
        "value": <string>
      },
      ...
    ],
    "settings": <CheckSettings>,
    "created": <timestamp>,
    "modified": <timestamp>
}
```

Description:

When adding a check, it's necessary to specify `target` and `job` as
well as at least one probe ID.

The `frequency` value specifies how often the check runs in milliseconds
(the value is not truly a "frequency" but a "period"). The minimum
acceptable value is 1 second (1000 ms), and the maximum is 120 seconds
(120000 ms).

The `timeout` value specifies the maximum running time for the check in
milliseconds. The minimum acceptable value is 1 second (1000 ms), and the
maximum 10 seconds (10000 ms).

The `ipVersion` value specifies whether the corresponding check will be
performed using IPv4 or IPv6. The "Any" value indicates that IPv6 should
be used, falling back to IPv4 if that's not available.

The `basicMetricsOnly` value specifies which set of metrics probes will collect. This is set to `true` by default in the UI which results in less active series or can be set to `false` for the advanced set. We maintain a [full list of metrics](https://github.com/grafana/synthetic-monitoring-agent/tree/main/internal/scraper/testdata) collected for each. 

The `alertSensitivity` value defaults to `none` if there are no alerts or can be set to `low`, `medium`, or `high` to correspond to the check [alert levels](https://grafana.com/docs/grafana-cloud/synthetic-monitoring/synthetic-monitoring-alerting/). 

The maximum number of labels that can be specified per check is 5. These
are applied, along with the probe-specific labels, to the outgoing
metrics. The names and values of the labels cannot be empty, and the
maximum length is 32 _bytes_.

For ping checks, the target must be a valid hostname or IP address.

For http checks the target must be a URL (http or https). The `header`
field specifies multiple headers that will be included in the request,
in the format that is appropriate for the HTTP version that should be
checked. The `cacheBustingQueryParamName` is the _name_ of the query
parameter that should be used to prevent the server from serving a
cached response (the value of this parameter is a random value generated
each time the check is performed).

For dns checks, the target must be a valid hostname (or IP address for
`PTR` records).

For tcp checks, the target must be of the form `<host>:<port>`,
where the host portion must be a valid hostname or IP address.

The returned value contains the ID assigned to this check, as well as
the ID of the tenant to which it belongs.

### /api/v1/check/update

Method: POST

Authorization required: yes

Content-type: application/json; charset=utf-8

Body:
```
{
    "id": <int>,
    "tenantId": <int>,
    "target": <string>,
    "job": <string>,
    "frequency": <int>,
    "timeout": <int>,
    "enabled": <boolean>,
    "alertSensitivity": <string>,
    "basicMetricsOnly": <boolean>,    
    "probes": [
      <int>,
      ...
    ],
    "labels": [
      {
        "name": <string>,
        "value": <string>
      },
      ...
    ],
    "settings": <CheckSettings>
}
```

Response:
```
{
    "id": <int>,
    "tenantId": <int>,
    "target": <string>,
    "job": <string>,
    "frequency": <int>,
    "timeout": <int>,
    "enabled": <boolean>,
    "alertSensitivity": <string>,
    "basicMetricsOnly": <boolean>,    
    "probes": [
      <int>,
      ...
    ],
    "labels": [
      {
        "name": <string>,
        "value": <string>
      },
      ...
    ],
    "settings": <CheckSettings>,
    "created": <timestamp>,
    "modified": <timestamp>
}
```

Description:

Update an existing check. The parameters are the same as those for adds, with
the exception that both the current check ID and the current tenant ID must be
provided. The response includes the updated value of the `modified` field.

### /api/v1/check/delete/:id:

Method: DELETE

Authorization required: yes

Response:
```
{
  "msg": "check deleted",
  "checkId": <int>
}
```

Description:

The check with the specified ID is deleted.

### /api/v1/check/list

Method: GET

Authorization required: yes

Response:
```
[
  <check>,
  ...
]
```

Description:

List all the checks for the tenant associated with the authorization token.

## Probes

### /api/v1/probe/add

Method: POST

Authorization required: yes

Content-type: application/json; charset=utf-8

Body:
```
{
  "name": <string>,
  "latitude": <float>,
  "longitude": <float>,
  "region": <string>,
  "labels": [
    {
      "name": <string>,
      "value": <string>
    },
    ...
  ]
}
```

Response:
```
{
  "probe": {
    "id": <int>,
      "tenantId": <int>,
      "name": <string>,
      "latitude": <float>,
      "longitude": <float>,
      "labels": [
      {
        "name": <string>,
        "value": <string>
      },
      ...
      ],
      "region": <string>,
      "public": <bool>,
      "online": <bool>,
      "onlineChange": <float>,
      "created": <float>,
      "modified": <float>
  },
  "token": <string>
}
```

Description:

Add a probe for the tenant associated with the authorization token. The
`public` field is reserved for use by Grafana Labs; the `public` field is
ignored if specified.

The response contains all the values for the newly added probe as well as a
token that they MUST present to the GRPC server in order to connect
successfully.

### /api/v1/probe/update

Method: POST

Query parameter: reset-token (optional, no value)

Authorization required: yes

Content-type: application/json; charset=utf-8

Body:
```
{
  "id": <int>,
  "tenantId": <int>,
  "name": <string>,
  "latitude": <float>,
  "longitude": <float>,
  "labels": [
  {
    "name": <string>,
    "value": <string>
  },
  ...
  ],
  "region": <string>,
}
```

Response:
```
{
  "probe": {
    "id": <int>,
      "tenantId": <int>,
      "name": <string>,
      "latitude": <float>,
      "longitude": <float>,
      "labels": [
      {
        "name": <string>,
        "value": <string>
      },
      ...
      ],
      "region": <string>,
      "public": <bool>,
      "online": <bool>,
      "onlineChange": <float>,
      "created": <float>,
      "modified": <float>
  },
  "token": <string>
}
```

Description:

This entry point is used to update an existing probe. Both the `id` and
`tenantId` values are required.

When the optional query parameter `reset-token` is included, the existing API
token is reset, and the new one is returned as part of the response.

### /api/v1/probe/list

Method: GET

Authorization required: yes

Response:
```
[
  {
    "id": <int>,
    "tenantId": <int>,
    "name": <string>,
    "latitude": <float>,
    "longitude": <float>,
    "labels": [
      {
        "name": <string>,
        "value": <string>
      },
      ...
    ],
    "region": <string>,
    "public": <bool>,
    "online": <bool>,
    "onlineChange": <float>,
    "created": <float>,
    "modified": <float>
  },
  ...
]
```

Description:

### /api/v1/probe/delete/:id:

Method: DELETE

Authorization required: yes

Response:
```
{
  "msg": "probe deleted",
  "probeID": <int>
}
```

Description:

The probe with the specified ID is deleted.

## Tenants

### /api/v1/tenant

Method: GET

Authorization required: yes

Response:
```
<tenant>
```

Description:

This entry point is used to obtain the information associated with the
authenticated tenant.

### /api/v1/tenant/update

Method: POST

Authorization required: yes

Content-type: application/json; charset=utf-8

Body:
```
{
  "id": <int>,
  "metricsRemote": {
    "name": <string>,
    "url": <string>,
    "username": <string>,
    "password": <string>
  },
  "eventsRemote": {
    "name": <string>,
    "url": <string>,
    "username": <string>,
    "password": <string>
  },
  "status": {
    "code": <int>,
    "reason": <string>
  }
}
```

Response:
```
{
	"msg": "tenant updated",
	"tenant": <tenant>
}
```

Description:

This entry point is used to update the metrics and events (logs) remote
information. The specified URLs are passed down to the probes, and they use
them to publish metrics and events.

If the metrics and events (logs) remote information is present, it's
updated in the existing tenant. The specified URLs are passed down to
the probes, and they use them to publish metrics and events.

If the status information is present, it's updated in the existing
tenant. Both the "code" and "reason" fields must be provided.

If the tenant status or the tenant's remote information changes, the
change is communicated to the probes. Specifically, if the tenant
becomes inactive, the associated checks are stopped; if the tenant
becomes active, the associated checks are started; if the remote
information changes, this is communicated to the probes so that they
refetch the authentication tokens as necessary.

### /api/v1/tenant/delete/:id:

Method: DELETE

Authorization required: yes

Response:
```
{
	"msg": "tenant deleted",
	"tenantId": <int>
}
```

Description:

This entry point is used to delete an existing tenant.

Before a tenant can be deleted all its checks and its probes must be deleted.
This is not done automatically.
