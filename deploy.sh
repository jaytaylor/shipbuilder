#!/usr/bin/env bash

##
# @author Jay Taylor [@jtaylor]
#
# @date 2013-07-11
#

if [ -z "$(which envdir)" ]; then
	echo 'fatal: no "envdir" binary found, make sure it is in a directory in your $PATH' 1>&2
	exit 1
fi

cd "$(dirname "$0")"

if ! [ -d './env' ]; then
    echo 'fatal: missing "env" configuration directory, see "Compilation" in the README' 1>&2
    exit 1
fi

envdir env go run deploy.go $*

