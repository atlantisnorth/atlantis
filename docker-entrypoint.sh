#!/bin/dumb-init /bin/sh
set -e

# Modified: https://github.com/hashicorp/docker-consul/blob/2c2873f9d619220d1eef0bc46ec78443f55a10b5/0.X/docker-entrypoint.sh

# https://github.com/runatlantis/helm-charts/issues/14
if [ -d /atlantis-data -a "$(stat -c %U:%G /atlantis-data 2>/dev/null)" != atlantis:atlantis ]; then
    echo "docker-entrypoint.sh: detected /atlantis-data wrong filesystem permissions" >&2
    echo "currently owned by $(stat -c %U:%G /atlantis-data 2>/dev/null), changing to atlantis:atlantis..." >&2
    chown -R atlantis:atlantis /atlantis-data
    echo "docker-entrypoint.sh: finished changing /atlantis-data filesystem permissions" >&2
fi

# If the user is trying to run atlantis directly with some arguments, then
# pass them to atlantis.
if [ "${1:0:1}" = '-' ]; then
    set -- atlantis "$@"
fi

# If the user is running an atlantis subcommand (ex. server) then we want to prepend
# atlantis as the first arg to exec. To detect if they're running a subcommand
# we take the potential subcommand and run it through atlantis help {subcommand}.
# If the output contains "atlantis subcommand" then we know it's a subcommand
# since the help output contains that string. For anything else (ex. sh)
# it won't contain that string.
# NOTE: We use grep instead of the exit code since help always returns 0.
if atlantis help "$1" 2>&1 | grep -q "atlantis $1"; then
    # We can't use the return code to check for the existence of a subcommand, so
    # we have to use grep to look for a pattern in the help output.
    set -- atlantis "$@"
fi

# If the current uid running does not have a user create one in /etc/passwd
if ! whoami &> /dev/null; then
  if [ -w /etc/passwd ]; then
    echo "${USER_NAME:-default}:x:$(id -u):0:${USER_NAME:-default} user:/home/atlantis:/sbin/nologin" >> /etc/passwd
  fi
fi

# If we're running as root and we're trying to execute atlantis then we use
# gosu to step down from root and run as the atlantis user.
# In OpenShift, containers are run as a random users so we don't need to use gosu.
if [[ $(id -u) == 0 ]] && [[ "$1" = 'atlantis' ]]; then
    # If requested, set the capability to bind to privileged ports before
    # we drop to the non-root user. Note that this doesn't work with all
    # storage drivers (it won't work with AUFS).
    if [ ! -z ${ATLANTIS_ALLOW_PRIVILEGED_PORTS+x} ]; then
        setcap "cap_net_bind_service=+ep" /bin/atlantis
    fi

    set -- gosu atlantis "$@"
fi

exec "$@"
