#!/usr/bin/env bash

##
# @author Jay Taylor [@jtaylor]
#
# @date 2013-07-11
#

cd "$(dirname "$0")"

# Verify that `go` and `envdir` (daemontools) dependencies are available.
test -z "$(which go)" && echo 'fatal: no "go" binary found, make go-lang is installed and available in a directory in $PATH' 1>&2 && exit 1
test -z "$(which envdir)" && echo 'fatal: no "envdir" binary found, make sure daemontools is installed and and available in $PATH' 1>&2 && exit 1

test ! -d './env' && echo 'fatal: missing "env" configuration directory, see "Compilation" in the README' 1>&2 && exit 1

if [ "$1" == '-f' ] || [ "$1" == '--fast' ]; then
    fastMode=1
    echo 'info: fast mode enabled'
fi


echo 'info: fetching dependencies'
# This finds all lines between:
# import (
#     ...
# )
# and appropriately filters the list down to the projects dependencies.  It also ignores any lines which start with "//", as those are comments.
dependencies=$(find src -wholename '*.go' -exec awk '{ if ($1 ~ /^import/ && $2 ~ /[(]/) { s=1; next; } if ($1 ~ /[)]/) { s=0; } if (s) print; }' {} \; | grep -v '^[^\.]*$' | tr -d '\t' | tr -d '"' | sed 's/^\. \{1,\}//g' | sort | uniq | grep -v '^[ \t]*\/\/')
for dependency in $dependencies; do
    echo "info:     retrieving: ${dependency}"
    if ! [ $fastMode ] || ! [ -d "$GOPATH/src/${dependency}" ]; then 
        go get -u $dependency
    else
        echo 'info:         -> already exists, skipping'
    fi
done


# Collect ldflags.
echo 'info: collecting compilation ldflags values from env/*'

IFS_BAK="${IFS}"
IFS=$'\n'
for pair in $(echo "$(date +%Y%m%d_%H%M%S) main.build
SB_SSH_HOST main.defaultSshHost
SB_SSH_KEY main.defaultSshKey
SB_AWS_KEY main.defaultAwsKey
SB_AWS_SECRET main.defaultAwsSecret
SB_AWS_REGION main.defaultAwsRegion
SB_S3_BUCKET main.defaultS3BucketName
SB_HAPROXY_CREDENTIALS main.defaultHaProxyCredentials
SB_HAPROXY_STATS main.defaultHaProxyStats
LXC_FS main.defaultLxcFs"); do
    envvar=$(echo "${pair}" | sed 's/^\([^ ]\{1,\}\).*$/\1/')
    govar=$(echo "${pair}" | sed 's/^[^ ]\{1,\} \(.*\)$/\1/')
    if [ -f "env/${envvar}" ] && [ -n $(cat "env/${envvar}") ]; then
        if [ -n "${ldflags}" ]; then
            ldflags="${ldflags} "
        fi
        ldflags="${ldflags}-X ${govar} $(cat "env/${envvar}")"
        echo "info:     found var ${envvar}, value=$(cat env/${envvar})"
    fi
    #if [ -z "${" ]; then
    #fi
done
IFS="${IFS_BAK}"
unset IFS_BAK


echo 'info: compiling project'
cd 'src'

if [ -n "${ldflags}" ]; then
    echo "info:     go build -o ../shipbuilder -ldflags ${ldflags}"
    go build -o ../shipbuilder -ldflags "${ldflags}"
else
    go build -o ../shipbuilder
fi

buildResult=$?

if [ $buildResult -eq 0 ]; then
    echo 'info:     build succeeded - the shipbuilder binary is located at ./shipbuilder'
else
    echo "error:     build failed, exited with status ${buildResult}" 1>&2
fi

exit $buildResult

