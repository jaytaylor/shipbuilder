#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

make \
    && sudo chown root:root ./shipbuilder/shipbuilder-linux \
    && sudo systemctl stop shipbuilder \
    && sudo mv ./shipbuilder/shipbuilder-linux /usr/bin/shipbuilder \
    && sudo chmod 755 /usr/bin/shipbuilder \
    && sudo systemctl start shipbuilder

