#!/usr/bin/env bash

##
# @author Jay Taylor [@jtaylor]
#
# @date 2013-07-11
#

set -o errexit
set -o pipefail
set -o nounset

cd "$(dirname "$0")"

# Verify that `go` and `envdir` (daemontools) dependencies are available.
test -z "$(which go)" && echo 'fatal: no "go" binary found, make sure go-lang is installed and available in a directory in $PATH' 1>&2 && exit 1
test -z "$(which envdir)" && echo 'fatal: no "envdir" binary found, make sure daemontools is installed and and available in $PATH' 1>&2 && exit 1

test ! -d './env' && echo 'fatal: missing "env" configuration directory, see "Compilation" in the README' 1>&2 && exit 1

echo 'info: fetching dependencies'
go get ./...

# Collect ldflags.
echo 'info: collecting compilation ldflags values from env/*'

ldflags=''
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
LXC_FS main.defaultLxcFs
ZFS_POOL main.defaultZfsPool"); do
    envvar=$(echo "${pair}" | sed 's/^\([^ ]\{1,\}\).*$/\1/')
    govar=$(echo "${pair}" | sed 's/^[^ ]\{1,\} \(.*\)$/\1/')
    if test -f "env/${envvar}" && test -n "$(head -n1 "env/${envvar}")"; then
        if test -n "${ldflags}"; then
            ldflags="${ldflags} "
        fi
        envval=$(head -n1 "env/${envvar}")
        ldflags="${ldflags}-X ${govar}=${envval}"
        echo "info:     found var ${envvar}, value=${envval}"
    fi
done
IFS="${IFS_BAK}"
unset IFS_BAK


echo 'info: compiling project'
cd 'src'

if test -n "${ldflags}"; then
    echo "info:     go build -o ../shipbuilder -ldflags ${ldflags}"
    go build -o ../shipbuilder -ldflags "${ldflags}"
else
    go build -o ../shipbuilder
fi

buildResult=$?

if test $buildResult -eq 0; then
    echo 'info:     build succeeded - the shipbuilder binary is located at ./shipbuilder'
else
    echo "error:     build failed, exited with status ${buildResult}" 1>&2
fi

exit $buildResult

