#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

##
# Prints core pkg LDFLAGS to stdout.
#
# Used by Makefile during build.
##

cd "$(dirname "$0")"

export GITHUB_DOMAIN=github.com
export GITHUB_ORG=jaytaylor
export GITHUB_REPO=shipbuilder

envdir env bash -c '
set -o errexit
set -o pipefail
set -o nounset
echo "\
-X ${GITHUB_DOMAIN}/${GITHUB_ORG}/${GITHUB_REPO}/pkg/core.DefaultHAProxyEnableNonstandardPorts=${SB_HAPROXY_ENABLE_NONSTANDARD_PORTS:-} \
-X ${GITHUB_DOMAIN}/${GITHUB_ORG}/${GITHUB_REPO}/pkg/core.DefaultHAProxyStats=${SB_HAPROXY_STATS:-} \
-X ${GITHUB_DOMAIN}/${GITHUB_ORG}/${GITHUB_REPO}/pkg/core.DefaultHAProxyCredentials=${SB_HAPROXY_CREDENTIALS:-} \
-X ${GITHUB_DOMAIN}/${GITHUB_ORG}/${GITHUB_REPO}/pkg/core.DefaultAWSKey=${SB_AWS_KEY:-} \
-X ${GITHUB_DOMAIN}/${GITHUB_ORG}/${GITHUB_REPO}/pkg/core.DefaultAWSSecret=${SB_AWS_SECRET:-} \
-X ${GITHUB_DOMAIN}/${GITHUB_ORG}/${GITHUB_REPO}/pkg/core.DefaultAWSRegion=${SB_AWS_REGION:-} \
-X ${GITHUB_DOMAIN}/${GITHUB_ORG}/${GITHUB_REPO}/pkg/core.DefaultS3BucketName=${SB_S3_BUCKET:-} \
-X ${GITHUB_DOMAIN}/${GITHUB_ORG}/${GITHUB_REPO}/pkg/core.DefaultSSHHost=${SB_SSH_HOST:-} \
-X ${GITHUB_DOMAIN}/${GITHUB_ORG}/${GITHUB_REPO}/pkg/core.DefaultSSHKey=${SB_SSH_KEY:-} \
-X ${GITHUB_DOMAIN}/${GITHUB_ORG}/${GITHUB_REPO}/pkg/core.DefaultLXCFS=${SB_LXC_FS:-} \
-X ${GITHUB_DOMAIN}/${GITHUB_ORG}/${GITHUB_REPO}/pkg/core.DefaultZFSPool=${SB_ZFS_POOL:-} \
"'

