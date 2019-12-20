#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

if [ "${1:-}" = '-v' ]; then
    verbose="$1"
	set -o xtrace
	shift
fi

cd "$(dirname "$0")/.."

export HOST="${HOST:-devsb3}"
rsync -azve ssh --delete --exclude=shipbuilder/*-* . "${HOST}:~/go/src/github.com/jaytaylor/shipbuilder/"
ssh "${HOST}" -- go/src/github.com/jaytaylor/shipbuilder/dev/deploy-server.sh ${verbose:-}

