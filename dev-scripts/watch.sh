#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

set -x

cd "$(dirname "$0")/.."

##
# When using bridged networking:
#
# fswatch --one-per-batch --recursive --monitor=fsevents_monitor -E '.' | xargs -n1 bash -c "rsync --delete -azve ssh . $(VBoxManage guestproperty get sb-server /VirtualBox/GuestInfo/Net/0/V4/IP | cut -d' ' -f2):~/go/src/github.com/jaytaylor/shipbuilder/"

##
# When using NAT networking:
#
fswatch --one-per-batch --recursive --monitor=fsevents_monitor -E '.' | xargs -n1 bash -c "rsync --delete -azve ssh . sb-server:~/go/src/github.com/jaytaylor/shipbuilder/"

