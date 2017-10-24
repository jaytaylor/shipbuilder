#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

set -x

cd "$(dirname "$0")/.."

fswatch --one-per-batch --recursive --monitor=fsevents_monitor -E '.' | xargs -n1 bash -c "rsync --delete -azve ssh . $(VBoxManage guestproperty get sb-server /VirtualBox/GuestInfo/Net/0/V4/IP | cut -d' ' -f2):~/go/src/github.com/jaytaylor/shipbuilder/"

