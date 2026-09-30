# This file is used by the makefile in order to obtain the version of the
# Grafana Build Tools image to use. This is *also* used by scripts/docker-run
# to obtain the same information. That means this file must be both a Makefile
# and a shell script. This is achieved by using the `VAR=value` syntax, which
# is valid in both Makefile and shell.

GBT_IMAGE=ghcr.io/grafana/grafana-build-tools:v1.48.0@sha256:f29499edc64eaab08a8bc724f43b3eb15717182a45f12e229d427039ad74d334
