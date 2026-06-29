# This file is used by the makefile in order to obtain the version of the
# Grafana Build Tools image to use. This is *also* used by scripts/docker-run
# to obtain the same information. That means this file must be both a Makefile
# and a shell script. This is achieved by using the `VAR=value` syntax, which
# is valid in both Makefile and shell.

GBT_IMAGE=ghcr.io/grafana/grafana-build-tools:v1.42.0@sha256:bc56136cd8768f0bd2eb3cb3811da61170b5f679c641bbb73e242aed4a064117
