# This file is used by the makefile in order to obtain the version of the
# Grafana Build Tools image to use. This is *also* used by scripts/docker-run
# to obtain the same information. That means this file must be both a Makefile
# and a shell script. This is achieved by using the `VAR=value` syntax, which
# is valid in both Makefile and shell.

GBT_IMAGE=ghcr.io/grafana/grafana-build-tools:v1.40.0@sha256:d35884b3d78065c4413b886ea31e9a659461a83b92480e0aab7a69e97d89bfa8
