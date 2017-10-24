#!/usr/bin/env bash

cd "$(dirname "$0")/.."

make clean deb && sudo dpkg -i dist/shipbuilder_*.deb && sudo systemctl restart shipbuilder

