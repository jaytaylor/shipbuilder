package core

import (
	"fmt"
	"text/template"

	"github.com/jaytaylor/shipbuilder/pkg/scripts"
)

const (
	PRE_RECEIVE = `#!/usr/bin/env bash

# set -x

set -o errexit
set -o pipefail
set -o nounset

#whoami
#ls -lah /git/test
#find /git/test
#rm -rf /tmp/test && cp -a /git/test /tmp/
echo '==========================================='
#find /tmp/test

while read oldrev newrev refname; do
    echo $newrev > $refname
    ` + EXE + ` pre-receive "$(pwd)" "${oldrev}" "${newrev}" "${refname}" # || exit 0
done`

	POST_RECEIVE = `#!/usr/bin/env bash

# set -x

set -o errexit
set -o pipefail
set -o nounset

while read oldrev newrev refname; do
    ` + EXE + ` post-receive "$(pwd)" "${oldrev}" "${newrev}" "${refname}"
done`

	LOGIN_SHELL = `#!/usr/bin/env bash
/usr/bin/envdir ` + ENV_DIR + ` /bin/bash`

	// # Cleanup old versions on the shipbuilder build box (only old versions, not the newest/latest version).
	// sudo -n lxc-ls --fancy | grep --only-matching '^[^ ]\+_v[0-9]\+ *STOPPED' | sed 's/^\([^ ]\+\)\(_v\)\([0-9]\+\) .*/\1 \3 \1\2\3/' | sort -t' ' -k 1,2 -g | awk -F ' ' '$1==app{ printf ",%s", $2 ; next } { app=$1 ; printf "\n%s %s", $1, $2 } END { printf "\n" }' | grep '^[^ ]\+ [0-9]\+,' | sed 's/,[0-9]\+$//' | awk -F ' ' '{ split($2,arr,",") ; for (i in arr) printf "%s_v%s\n", $1, arr[i] }' | xargs -n1 -IX bash -c 'attempts=0; rc=1; while [ $rc -ne 0 ] && [ $attempts -lt 10 ] ; do echo "rc=${rc}, attempts=${attempts} X"; sudo -n lxc-destroy -n X; rc=$?; attempts=$(($attempts + 1)); done'

	// # Cleanup old zfs container volumes not in use (primarily intended to run on nodes and sb server).
	// containers=$(sudo -n lxc-ls --fancy | sed "1,2d" | cut -f1 -d" ") ; for x in $(sudo -n zfs list | sed "1d" | cut -d" " -f1); do if [ "${x}" = "tank" ] || [ "${x}" = "tank/git" ] || [ "${x}" = "tank/lxc" ]; then echo "skipping bare tank, git, or lxc: ${x}"; continue; fi; if [ -n "$(echo $x | grep '@')" ]; then search=$(echo $x | sed "s/^.*@//"); else search=$(echo $x | sed "s/^[^\/]\+\///"); fi; if [ -z "$(echo -e "${containers}" | grep "${search}")" ]; then echo "destroying non-container zfs volume: $x" ; sudo -n zfs destroy $x; fi; done

	// # Cleanup empty container dirs.
	// for dir in $(find /var/lib/lxc/ -maxdepth 1 -type d | grep '.*_v[0-9]\+_.*_[0-9]\+'); do if test "${dir}" = '.' || test -z "$(echo "${dir}" | sed 's/\/var\/lib\/lxc\///')"; then continue; fi; count=$(find "${dir}/rootfs/" | head -n 3 | wc -l); if test $count -eq 1; then echo $dir $count; echo sudo -n rm -rf $dir; fi; done

	// ZFS MAINTENANCE NG:
	//
	// Prune highest version container from list:
	// echo "(list of containers delimited by newlines)" | \
	//     sed 's/\(.*_v\)\([0-9]\+\)\(.*\)/\2 \1 \3/' | sort -r -n | sed '1d' | sed 's/\(.*\) \(.*\) \(.*\)/\2\1\3/'
	//
	// Reliably destroy container and associated zfs volumes:
	// echo "(list of containers delimited by newlines)" | \
	//     xargs -n1 -IX bash -c '( sudo -n lxc-destroy -n X || sudo -n lxc-destroy -n X || sudo -n lxc-destroy -n X ) && ( sudo -n zfs destroy tank/X; sudo -n zfs destroy tank/$(echo X | sed "s/\([^_]\+\).*/\1/")@X || :)'
	//
	// echo optic_v576_worker_10010 | head -n1 | xargs -n1 -IX bash -c 'echo X ; ( sudo -n zfs destroy tank/X; sudo -n zfs destroy tank/$(echo X | sed "s/\([^_]\+\).*/\1/")@X || : ) && test $(sudo -n find /var/lib/lxc/X/rootfs -maxdepth 1 | wc -l) -lt 2 && sudo -n rm -rf /var/lib/lxc/X && echo "destroyed X" || echo "failed to eradicate X"'
	//
	// sudo -n lxc-ls --fancy | grep '_v[0-9]\+.*STOPPED' | cut -f1 -d' ' | xargs -n1 -IX bash -c 'echo X ; ( sudo -n zfs destroy tank/X; sudo -n zfs destroy tank/$(echo X | sed "s/\([^_]\+\).*/\1/")@X || : ) && test $(sudo -n find /var/lib/lxc/X/rootfs -maxdepth 1 | wc -l) -lt 2 && sudo -n rm -rf /var/lib/lxc/X && echo "destroyed X" || echo "failed to eradicate X"'

	ZFS_MAINTENANCE = `#!/usr/bin/env bash

# Cleanup old versions on the shipbuilder build box (only old versions, not the newest/latest version).

## How this filter pipe works
# ---------------------------
# First, captures LXC state as JSON and initiates filtration by desired
# container state.  Using principle of word atom splitting to sort by app by version.
# Then converts to sequence of comma-delimited:
#
#     app-name v1,v2,vN...
#
# Preserves only the latest version (at very end of sequence), and finally
# rejoins back to app-version format.

containerLxcState="$(sudo -n ` + LXC_BIN + ` list --format=json)"
containerPreserveVersionsRe=$(
    echo -n "${containerLxcState}" \
    | jq -r '.[] | select(.status == "Stopped") | .name' \
    | grep --only-matching '^[^ ]\+` + DYNO_DELIMITER + `v[0-9]\+.*$' \
    | sed 's/^\([^ ]\+\)\(` + DYNO_DELIMITER + `v\)\([0-9]\+\)\(.*\)$/\1 \3 \1\2\3\4/' \
    | sort -t' ' -k 1,2 -g \
    | awk -F ' ' '$1==app{ printf ",%s", $2 ; next } { app=$1 ; printf "\n%s %s", $1, $2 } END { printf "\n" }' \
    | sed 's/\([0-9]\+,\)*\([0-9]\+\)$/\2/' \
    | awk -F ' ' '{ split($2,arr,",") ; for (i in arr) printf "%s` + DYNO_DELIMITER + `v%s\n", $1, arr[i] }' \
    | uniq \
    | tr '\n' ' ' \
    | sed 's/ /\\|/g' | sed 's/\\|$//' \
)

containerDestroyVersions=$(
    echo -n "${containerLxcState}" \
    | jq -r '.[] | select(.status == "Stopped") | .name' \
    | grep --only-matching '^[^ ]\+` + DYNO_DELIMITER + `v[0-9]\+.*$' \
    | sed 's/^\([^ ]\+\)\(` + DYNO_DELIMITER + `v\)\([0-9]\+\)\(.*\)$/\1 \3 \1\2\3\4/' \
    | sort -t' ' -k 1,2 -g \
    | awk -F ' ' '$1==app{ printf ",%s", $2 ; next } { app=$1 ; printf "\n%s %s", $1, $2 } END { printf "\n" }' \
    | grep '^[^ ]\+ [0-9]\+,' \
    | sed 's/,[0-9]\+$//' \
    | awk -F ' ' '{ split($2,arr,",") ; for (i in arr) printf "%s` + DYNO_DELIMITER + `v%s\n", $1, arr[i] }' \
    | uniq
)


imageLxcState="$(sudo ` + LXC_BIN + ` image list)"
imagePreserveVersionsRe=$(
    echo -n "${imageLxcState}" \
    | sed '1,2d' \
    | grep '^| ' \
    | sed 's/^\(| \([^ ]\+\)\)\? *|.*/\2/g' \
    | grep --only-matching '^[^ ]\+` + DYNO_DELIMITER + `v[0-9]\+.*$' \
    | sed 's/^\([^ ]\+\)\(` + DYNO_DELIMITER + `v\)\([0-9]\+\)\(.*\)$/\1 \3 \1\2\3\4/' \
    | sort -t' ' -k 1,2 -g \
    | awk -F ' ' '$1==app{ printf ",%s", $2 ; next } { app=$1 ; printf "\n%s %s", $1, $2 } END { printf "\n" }' \
    | sed 's/\([0-9]\+,\)*\([0-9]\+\)$/\2/' \
    | awk -F ' ' '{ split($2,arr,",") ; for (i in arr) printf "%s` + DYNO_DELIMITER + `v%s\n", $1, arr[i] }' \
    | uniq \
    | tr '\n' ' ' \
    | sed 's/ /\\|/g' | sed 's/\\|$//' \
)

imageDestroyVersions=$(
    echo -n "${imageLxcState}" \
    | sed '1,2d' \
    | grep '^| ' \
    | sed 's/^\(| \([^ ]\+\)\)\? *|.*/\2/g' \
    | grep --only-matching '^[^ ]\+` + DYNO_DELIMITER + `v[0-9]\+.*$' \
    | sed 's/^\([^ ]\+\)\(` + DYNO_DELIMITER + `v\)\([0-9]\+\)\(.*\)$/\1 \3 \1\2\3\4/' \
    | sort -t' ' -k 1,2 -g \
    | awk -F ' ' '$1==app{ printf ",%s", $2 ; next } { app=$1 ; printf "\n%s %s", $1, $2 } END { printf "\n" }' \
    | grep '^[^ ]\+ [0-9]\+,' \
    | sed 's/,[0-9]\+$//' \
    | awk -F ' ' '{ split($2,arr,",") ; for (i in arr) printf "%s` + DYNO_DELIMITER + `v%s\n", $1, arr[i] }' \
    | uniq
)

# Define function to destroy a container.
function destroyContainer() {
    name="$1"
    echo "Destroying stopped container name=${name}"

    #sudo -n zfs destroy tank/${name} 1>/dev/null 2>/dev/null || \
    #    sudo -n zfs destroy tank/${name} 1>/dev/null 2>/dev/null || \
    #    sudo -n zfs destroy tank/${name}

    #sudo -n zfs destroy tank/$(echo ${name} | grep --only-matching '^[^_]\+')@${name} 1>/dev/null 2>/dev/null || \
    #    sudo -n zfs destroy tank/$(echo ${name} | grep --only-matching '^[^_]\+')@${name} 1>/dev/null 2>/dev/null || \
    #    sudo -n zfs destroy tank/$(echo ${name} | grep --only-matching '^[^_]\+')@${name}

    #sudo -n zfs destroy tank/$(echo ${name} | grep --only-matching '^[^_]\+-v[0-9]\+')@${name} 1>/dev/null 2>/dev/null || \
    #    sudo -n zfs destroy tank/$(echo ${name} | grep --only-matching '^[^_]\+-v[0-9]\+')@${name} 1>/dev/null 2>/dev/null || \
    #    sudo -n zfs destroy tank/$(echo ${name} | grep --only-matching '^[^_]\+-v[0-9]\+')@${name}

    #test $(find /var/lib/lxc/${name}/rootfs/ -maxdepth 1 | wc -l) -eq 1 && sudo -n rm -rf "/var/lib/lxc/${name}" #|| echo "FAILED TO DESTROY container=${name}"

    ` + LXC_BIN + ` delete --force "${name}"
}
# Export the fn so it can be used in a xargs .. bash -c '<here>'
export -f destroyContainer

## Function to destroy all non-container zfs volumes.
#function destroyNonContainerVolumes() {
#    zfsContainerPattern='^tank\/\([a-zA-Z0-9-]\+@\)\?[a-zA-Z0-9-]\+_\(v[0-9]\+\(_.\+_[0-9]\+\)\?\|console_[a-zA-Z0-9]\+\)$'

#    # Notice the spaces around the edges so we can match [:SPACE:][precise-container-name][:SPACE:]
#    containers=" $(echo "${lxcLs}" | sed '1,2d' | sed 's/ \+/ /g' | cut -d' ' -f1 | tr '\n' ' ') "
#    candidateZfsVolumes="$(sudo -n zfs list | sed '1d' | cut -d' ' -f1 | grep "${zfsContainerPattern}" | sed 's/^\([^\/]\+\/\+\)\?\([^@]\+@\)\?//' | sort | uniq)"
#    for searchContainerName in $candidateZfsVolumes; do
#        if [ -z "${searchContainerName}" ] || [ -n "$(echo "${searchContainerName}" | grep '^\(tank\/\)\?\(git\|lxc\)$')" ]; then
#            echo "skipping bare tank, git, or lxc: ${searchContainerName}"
#            continue
#        fi
#        if [ -n "$(echo " ${containers} " | grep " ${searchContainerName} ")" ]; then
#            echo "skipping container=${searchContainerName} because it is an lxc container"
#        elif ! test -d "/var/lib/lxc/${searchContainerName}" ; then
#            destroyContainer "${searchContainerName}"
#        fi
#    done
#}

## Cleanup any straggler containers first so that versioned app containers can be successfully removed next (note: candidates must be in a stopped state).
#function destroyStragglerContainers() {
#    echo "${lxcLs}" | \
#        grep '^[a-zA-Z0-9-]\+_\(v[0-9]\+\(_.\+_[0-9]\+\)\?\|console_[a-zA-Z0-9]\+\).*STOPPED' | \
#        cut -d' ' -f1 | \
#        grep -v "^\(${preserveVersionsRe}\)$" | \
#        xargs -n1 -IX bash -c 'destroyContainer X'
#}

# Destroy old app versions.
function destroyOldAppVersions() {
    for container in $(` + LXC_BIN + ` list --format=csv | cut -d ',' -f 1) ; do
        for destroyVersion in ${destroyVersions} ; do
            if [[ "$container" =~ ^$destroyVersion ]] ; then
                destroyContainer "${container}"
                break
            fi
        done
    done
}

#destroyNonContainerVolumes

#destroyStragglerContainers

destroyOldAppVersions

#destroyNonContainerVolumes

## Cleanup any empty container directories.
#for dir in $(find /var/lib/lxc/ -maxdepth 1 -type d | grep '[a-zA-Z0-9-]\+_\(v[0-9]\+\(_.\+_[0-9]\+\)\?\|console_[a-zA-Z0-9]\+\)'); do
#    if test "${dir}" = '.' || test -z "$(echo "${dir}" | sed 's/\/var\/lib\/lxc\///')"; then
#        continue
#    fi
#    count=$(find "${dir}/rootfs/" | head -n 3 | wc -l)
#    if test $count -eq 1; then
#        echo $dir $count
#        sudo -n rm -rf $dir
#    fi
#done

exit $?`

	// LXDCompatScript updates the LXD systemd service definition to protect
	// against /var/lib/lxd path conflicts between LXD and shipbuilder.
	LXDCompatScript = `
#!/usr/bin/env python
# -*- coding: utf-8 -*-

"""
This program modifies the LXD systemd definition to ensure two things:

    1. The LXD service can safely start.

    2. Path /var/lib/lxd remains usable for shipbuilder to reference as needed.

This is needed because LXD fails to start due to mounting conflicts if
/var/lib/lxd exists during service initialization.  After it starts, the
temporary mounts are removed and /var/lib/lxd becoes available again for
shipbuilder to use.
"""

import os
import re
import subprocess
import sys

systemd_file = '/etc/systemd/system/snap.lxd.daemon.service'

pre = '''ExecStartPre=/bin/bash -c 'set -o errexit && test -e /var/lib/lxd && mv /var/lib/lxd /var/lib/lxd.starting' '''.strip()
start = '''ExecStart=/usr/bin/snap run lxd.daemon'''
post = '''ExecStartPost=/bin/bash -c 'set -o errexit && sleep 5 && test -e /var/lib/lxd.starting && mv /var/lib/lxd.starting /var/lib/lxd' '''.strip()

start_expr = re.compile(r'''(.*\n)^(%s)$(\n.*)''' % (start,), re.M | re.S)

prepost_expr = re.compile(r'''.*^%s$\n^%s$\n^%s$.*''' % (pre, start, post), re.M | re.S)

def requireRoot(argv):
    # Require root user.
    if os.environ.get('USER', '') != 'root':
        sys.stderr.write('FATAL: %s must be run under root user\n' % (argv[0],))
        sys.exit(1)

def main(argv):
    requireRoot(argv)

    with open(systemd_file, 'r') as fh:
        contents = fh.read()

    if prepost_expr.match(contents):
        print('INFO: Nothing to be done; Pre- and Post- ExecStart customizations already exist in "%s"' % (systemd_file,))
        return 0

    if not start_expr.match(contents):
        print('ERROR: No matching ExecStart block found in %s' % (systemd_file,))
        return 2

    revised = start_expr.sub(r'\1%s\n\2\n%s\3' % (pre, post), contents)
    temp_file = '%s.tmp' % (systemd_file,)
    with open(temp_file, 'w') as fh:
        fh.write(revised)
    os.rename(temp_file, systemd_file)
    print('INFO: Applied Pre- and Post- ExecStart customizations to "%s"' % (systemd_file,))
    subprocess.check_call('systemctl daemon-reload'.split(' '))
    print('INFO: Successfully reloaded systemd')
    return 0

if __name__ == '__main__':
    sys.exit(main(sys.argv))
`

	// pyIptables is a python fragment with a collection of functions used by
	// postdeploy.py and shutdown_container.py.
	pyIptables = `
import re
import shlex

def newIpTablesCmd(actionLetter, commandFragment):
    command = '/sbin/iptables --table nat -' + actionLetter + ' ' + commandFragment
    log('iptables command: %s' % (command,))
    p = subprocess.Popen(
        shlex.split(command),
        stderr=sys.stderr,
        stdout=sys.stdout,
    )
    return p

def iptablesCommentPrefix(name):
    """
    Generates the iptables comment prefix which corresponds with specified name.
    """
    if name == 'remote':
        commentPrefix = 'Shipbuilder remote NAT for app-container='
    elif name == 'local':
        commentPrefix = 'Shipbuilder local NAT for app-container='
    elif name == 'unicast':
        commentPrefix = 'Shipbuilder local NAT unicast masquerade'
    else:
        raise Exception('Unrecognized iptables comment prefix "%s" requested' % (name,))
    return commentPrefix

def portForwardCommandFragments(includePostrouting, ip, port, container):
    """
    Fragments which are built into commands like:
         iptables -t nat -A PREROUTING -m tcp -p tcp --dport {port} -j DNAT --to-destination {ip}:{port} {commentFlag}
    or:
         iptables -t nat -C PREROUTING -m tcp -p tcp --dport {port} -j DNAT --to-destination {ip}:{port} {commentFlag}

    Also see this stackoverflow for details on how local forwarding now works:

        https://serverfault.com/a/823145/122599

    @param includePostrouting bool When False, the POSTROUTING UNICAST
        MASQUERADE rule will be omitted.
    """
    fragments = re.sub(
        ' +',
        ' ',
        '''
PREROUTING  -m tcp      -p tcp --dport %(port)s -j DNAT --to-destination %(ip)s:%(port)s                          -m comment --comment '%(remote)s%(container)s'
OUTPUT      -m addrtype --src-type LOCAL --dst-type LOCAL -p tcp --dport %(port)s -j DNAT --to-destination %(ip)s -m comment --comment '%(local)s%(container)s'
POSTROUTING -m addrtype --src-type LOCAL --dst-type UNICAST -j MASQUERADE                                         -m comment --comment '%(unicast)s'
        '''.strip()
            % {
                'ip': ip,
                'port': port,
                'container': container,
                'remote': iptablesCommentPrefix('remote'),
                'local': iptablesCommentPrefix('local'),
                'unicast': iptablesCommentPrefix('unicast'),
            },
    ).split('\n')
    if not includePostrouting:
        fragments = fragments[0:2]
    return fragments

def portForward(action, container, ip, port):
    """
    Adds or removes a port forwarding rule for an IP / port combination.

    If @action is 'remove' and @ip is an empty string then automatic resolution
    will be attempted by inspecting the iptables rules corresponding to port=@port.
    """
    assert action in ('add', 'remove'), 'Action must be one of: add, remove'
    actionLetter = 'A' if action == 'add' else 'D'

    if action == 'remove' and len(ip) == 0:
        # Attempt automatic resolution of IP.
        ip = subprocess.check_output([
            'bash',
            '-c',
            '''set -o errexit ; set -o pipefail ; /sbin/iptables --table nat --list | {{ grep '^DNAT.* tcp dpt:{}' || : ; }} | head -n1 | sed 's/^.* to://' | cut -d ':' -f 1'''.format(port),
        ]).strip()
        if len(ip) == 0:
            # No rules need to be removed.
            return

    # Sometimes iptables is being run too many times at once on the same box, and will give an error like:
    #     iptables: Resource temporarily unavailable.
    #     exit status 4
    # We try to detect any such occurrence, and up to N times we'll wait for a moment and retry.
    attempts = 0

    for fragment in portForwardCommandFragments(action == 'add', ip, port, container):
        while True:
            child = newIpTablesCmd('C', fragment)
            child.communicate()
            statusCode = child.returncode
            if statusCode is 0:
                # Rule already exists!
                if action == 'add':
                    break
                else:
                    # Needs removal.
                    child = newIpTablesCmd(actionLetter, fragment)
                    child.communicate()
                    statusCode = child.returncode
                    if statusCode is 0:
                        break
                    log('iptables: {action} rule for ip:port={ip}:{port} failed with exit status code {statusCode} ({attempts} previous attempts)'
                        .format(action=action, ip=ip, port=port, statusCode=statusCode, attempts=attempts))
                    attempts += 1
                    time.sleep(0.5)
                    continue

            if statusCode is 1:
                # Rule not found.
                if action == 'remove':
                    break
                # Else the rule needs to be added.
                child = newIpTablesCmd(actionLetter, fragment)
                child.communicate()
                statusCode = child.returncode
                if statusCode is 0:
                    break
                log('iptables: {action} rule for ip:port={ip}:{port} failed with exit status code {statusCode} ({attempts} previous attempts)'
                    .format(action=action, ip=ip, port=port, statusCode=statusCode, attempts=attempts))
                attempts += 1
                time.sleep(0.5)
                continue
            elif statusCode == 4 and attempts < 40:
                log('iptables: Resource temporarily unavailable (exit status 4), retrying.. ({0} previous attempts)'.format(attempts))
                attempts += 1
                time.sleep(0.5)
                continue
            else:
                raise subprocess.CalledProcessError(statusCode, 'iptables failure; no handler for exit status code {0}'.format(statusCode))
`

	// AutoIPTablesScript is the automatic IP tables fixer script to allow
	// containers running on slaves to survive reboots.
	AutoIPTablesScript = `#!/usr/bin/env python
# -*- coding: utf-8 -*-

` + pyIptables + `

import os
import subprocess
import sys

log = lambda message: sys.stdout.write('%s\n' % (message,))

dynoDelimiter = '-'

lsCmd = '''lxc list | sed 1,3d | grep -v '^+' | awk '$4 == "RUNNING" { print $2 " " $6 }' '''.strip()

def requireRoot(argv):
    # Require root user.
    if os.environ.get('USER', '') != 'root':
        sys.stderr.write('FATAL: %s must be run under root user\n' % (argv[0],))
        sys.exit(1)

def main(argv):
    requireRoot(argv)

    #out = subprocess.check_output(['bash', '-c', '''set -o errexit && set -o pipefail && %s''' % (lsCmd,)], shell=True)
    out = subprocess.check_output(['bash', '-c', '''set -o xtrace && set -o errexit && set -o pipefail && %s''' % (lsCmd,)]).strip()

    print('----')
    for line in out.split('\n'):
        try:
            container, ip = line.split(' ', 2)
            app, version, process, port = container.rsplit(dynoDelimiter, 3) # Format is app-version-process-port.
            portForward('add', container, ip, port)
            print('app=%s v=%s process=%s ort=%s' % (app, version, process, port))
        except ValueError:
            print('WARNING: Ignoring unrecognized container/ip %s' % (line,))
    print('----')

if __name__ == '__main__':
    sys.exit(main(sys.argv))
`
)

// TODO TODO TODO: ADD IPTABLES-BASED PORT CHECK TO THIS.
var POSTDEPLOY = `#!/usr/bin/python -u
# -*- coding: utf-8 -*-

import os
import stat
import subprocess
import sys
import time

defaultLxcFs='''` + DefaultLXCFS + `'''
lxcDir='''` + LXC_DIR + `'''
lxcBin='''` + LXC_BIN + `'''
zfsContainerMount='''` + ZFS_CONTAINER_MOUNT + `'''
dynoDelimiter = '''` + DYNO_DELIMITER + `'''
defaultSshHost = '''` + DefaultSSHHost + `'''
envDir = '''` + ENV_DIR + `'''
container = None
log = lambda message: sys.stdout.write('[{0}] {1}\n'.format(container, message))

` + pyIptables + `

def getIp(name):
    ip = subprocess.check_output([
        'bash',
        '-c',
        ''' ` + bashLXCIPWaitCommand + ` '''.strip() % (name,),
    ]).strip()
    if ip:
        return ip
    return ''

def enableRouteLocalNet():
    """
    Set sysctl -w net.ip4v.conf.all.route_localnet=1 to ensure iptables port
    forwarding rules work.
    """
    out = subprocess.check_output('sysctl --binary net.ipv4.conf.all.route_localnet'.split(' '))
    if out == '1':
        return
    subprocess.check_call('sysctl --write --quiet net.ipv4.conf.all.route_localnet=1'.split(' '),
        stdout=sys.stdout,
        stderr=sys.stderr,
    )
    return

def cloneContainer(app, container, version, check=True):
    log('cloning container: {0}'.format(container))
    fn = subprocess.check_call if check else subprocess.call
    return fn(
        [lxcBin, 'init', app + dynoDelimiter + version, container],
        stdout=sys.stdout,
        stderr=sys.stderr,
    )

def startContainer(container, check=True):
    log('starting container: {}'.format(container))
    fn = subprocess.check_call if check else subprocess.call
    return fn(
        [lxcBin, 'start', container],
        stdout=sys.stdout,
        stderr=sys.stderr,
    )

def mountContainerFs(container):
    if defaultLxcFs != 'zfs':
        return
    subprocess.check_call(
        ['sudo', '-n', 'zfs', 'mount', zfsContainerMount.strip('/') + '/' + container],
        stdout=sys.stdout,
        stderr=sys.stderr,
    )

def unmountContainerFs(container):
    if defaultLxcFs != 'zfs':
        return
    subprocess.check_call(
        ['sudo', '-n', 'zfs', 'umount', zfsContainerMount.strip('/') + '/' + container],
        stdout=sys.stdout,
        stderr=sys.stderr,
    )

def showHelpAndExit(argv):
    message = '''usage: {} [container-name]
       of the form: [app]_[version]_[process]_[port]

       For example, here is how you would boot a container with the following attributes:

           {
               "app-name": "myApp",
               "version-tag": "v1337",
               "process-type": "web",
               "port-forward": "10001"
           }

       $ {} myApp-v1337-web-10001
'''.format(argv[0])
    print(message)
    sys.exit(0)

def requireRoot(argv):
    # Require root user.
    if os.environ.get('USER', '') != 'root':
        sys.stderr.write('FATAL: %s must be run under root user\n' % (argv[0],))
        sys.exit(1)

def validateMainArgs(argv):
    if len(argv) != 2:
        sys.stderr.write('{} error: missing required argument: container-name\n'.format(sys.argv))
        sys.exit(1)

def parseMainArgs(argv):
    validateMainArgs(argv)
    container = argv[1]
    app, version, process, port = container.rsplit(dynoDelimiter, 3) # Format is app-version-process-port.
    return (container, app, version, process, port)

def main(argv):
    global container

    if len(argv) > 1 and argv[1] in ('-h', '--help', 'help'):
        showHelpAndExit(argv)

    requireRoot(argv)

    container, app, version, process, port = parseMainArgs(argv)

    # For safety, even though it's unlikely, try to kill/shutdown any existing container with the same name.
    subprocess.call([lxcBin + ' stop --force {0} 1>&2 2>/dev/null'.format(container)], shell=True)
    subprocess.call([lxcBin + ' delete --force {0} 1>&2 2>/dev/null'.format(container)], shell=True)

    # Clone the specified container.
    cloneContainer(app, container, version)

    log('creating run script for app "{0}" with process type={1}'.format(app, process))
    # NB: The curly braces are kinda crazy here, to get a single '{' or '}' with python.format(), use double curly
    # braces.
    host = defaultSshHost
    runScript = '''#!/usr/bin/env bash

# TODO: enable errexit and pipefail.
# set -o errexit
# set -o pipefail
set -o nounset

ip addr show eth0 | grep 'inet.*eth0' | awk '{{print $2}}' > /app/ip

rm -rf /tmp/log

cd /app/src

__DEBUG=''

if [ -f ../env/SB_DEBUG ] ; then
    export SB_DEBUG="$(cat ../env/SB_DEBUG)"
    if [[ "${{SB_DEBUG:-}}" =~ ^(1|[tT]([rR][uU][eE])?|[yY]([eE][sS])?)$ ]] ; then
        __DEBUG='set -o xtrace ; '
    fi
fi

echo '{port}' > ../env/PORT
while read line || [ -n "${{line}}" ]; do
    # Convert app process name from snake to camelCase.
    process="$(echo "${{line%%:*}}" | sed 's/[_-]\+\([a-zA-Z0-9]\)/\U\1/g')"
    command="${{line#*: }}"
    if [ "${{process}}" = "{process}" ]; then
        envdir {envDir} /bin/bash -c "${{__DEBUG}}export PATH=\"$(find /app/.shipbuilder -type d -wholename '*bin' -maxdepth 2):${{PATH}}\" ; set -o errexit ; set -o pipefail ; ( ${{command}} ) 2>&1 | /app/` + BINARY + ` logger --host={host} --app={app} --process={process}.{port}"
    fi
done < Procfile'''.format(port=port, host=host.split('@')[-1], process=process, app=app, envDir=envDir)

    mountContainerFs(container)

    runScriptFileName = lxcDir + '/{0}/rootfs/app/run'.format(container)
    with open(runScriptFileName, 'w') as fh:
        fh.write(runScript)

    # Chmod to be executable.
    st = os.stat(runScriptFileName)
    os.chmod(runScriptFileName, st.st_mode | stat.S_IEXEC)

    unmountContainerFs(container)

    startContainer(container)

    log('waiting for container to boot and report IP-address')
    numChecks = 45
    # Allow container to bootup.
    ip = None
    for _ in xrange(numChecks):
        time.sleep(1)
        try:
            ip = getIp(container)
            if ip:
                # IP obtained!
                break
        except:
            continue

    if ip:
        log('found ip: {0}'.format(ip))
        portForward('remove', container, '', port)
        portForward('add', container, ip, port)

        if process == 'web':
            log('waiting for web-server to start up')
            startedTs = time.time()
            maxSeconds = 60
            while True:
                try:
                    subprocess.check_call([
                        '/usr/bin/curl',
                        '--silent',
                        '--output', '/dev/null',
                        '--write-out', '%{http_code} %{url_effective}\n',
                        '{0}:{1}/'.format(ip, port),
                    ], stderr=sys.stderr, stdout=sys.stdout)
                    break

                except subprocess.CalledProcessError, e:
                    if time.time() - startedTs > maxSeconds:
                        sys.stderr.write('- error: curl http check failed, {0}\n'.format(e))
                        subprocess.check_call(['/tmp/shutdown_container.py', container, 'skip-stop'])
                        sys.exit(1)
                    else:
                        time.sleep(1)

    else:
        sys.stderr.write('- error: failed to retrieve container ip\n')
        subprocess.check_call(['/tmp/shutdown_container.py', container, 'skip-stop'])
        sys.exit(1)

main(sys.argv)`

var SHUTDOWN_CONTAINER = `#!/usr/bin/python -u
# -*- coding: utf-8 -*-

import os
import subprocess
import sys
import time

lxcBin='''` + LXC_BIN + `'''
DefaultLXCFS = '''` + DefaultLXCFS + `'''
DefaultZFSPool = '''` + DefaultZFSPool + `'''
dynoDelimiter = '''` + DYNO_DELIMITER + `'''
container = None
log = lambda message: sys.stdout.write('[{0}] {1}\n'.format(container, message))

` + pyIptables + `

def retriableCommand(*command):
    for _ in range(0, 30):
        try:
            return subprocess.check_call(command, stdout=sys.stdout, stderr=sys.stderr)
        except subprocess.CalledProcessError, e:
            if 'dataset is busy' in str(e):
                time.sleep(0.5)
                continue
            else:
                raise e

def showHelpAndExit(argv, ok=True):
    message = '''usage: {} [container-name] [?"skip-stop", ?"iptables-only"]
       where "container-name" is of the form: [app]-v[version]-[process]-[port]

       "skip-stop" will only go about destroying the container (not shutting
        it down).

       "iptables-only" will skip stop / destroy steps and only attempt to purge
        the iptables rules corresponding with the container.

       For example, here is how you would terminate a container with the
       following attributes:

           {{
               "app-name": "myApp",
               "version-tag": "v1337",
               "process-type": "web",
               "port-forward": "10001"
           }}

       $ {} myApp-v1337-web-10001
'''.format(argv[0], argv[0])
    print(message)
    sys.exit(0 if ok else 1)

def requireRoot(argv):
    # Require root user.
    if os.environ.get('USER', '') != 'root':
        sys.stderr.write('FATAL: %s must be run under root user\n' % (argv[0],))
        sys.exit(1)

def validateMainArgs(argv):
    if len(argv) < 2 or (len(argv) > 2 and not all(map(lambda arg: arg in ('skip-stop', 'iptables-only'), argv[2:]))):
        showHelpAndExit(argv, False)

def parseMainArgs(argv):
    validateMainArgs(argv)
    container = argv[1]
    app, version, process, port = container.rsplit(dynoDelimiter, 3) # Format is app-version-process-port.
    return (container, app, version, process, port)

def main(argv):
    global container

    if len(argv) > 1 and argv[1] in ('-h', '--help', 'help'):
        showHelpAndExit(argv, True)

    requireRoot(argv)

    container, app, version, process, port = parseMainArgs(argv)
    skipStop = len(argv) > 2 and 'skip-stop' in argv[2:]
    iptablesOnly = len(argv) > 2 and 'iptables-only' in argv[2:]

    ip = subprocess.check_output([
        'bash',
        '-c',
        '''` + bashSafeEnvSetup + `%(lxc)s list --format json | jq -r '.[] | select(.name == "%(container)s").state.network.eth0.addresses[0].address' '''.strip() \
            % {'lxc': lxcBin, 'container': container},
    ]).strip()
    if not ip or ip == 'null':
        ip = subprocess.check_output([
            'bash',
            '-c',
            '''` + bashSafeEnvSetup + `iptables-save | (grep '%s%s' || true) | (` + bashGrepIP + ` || true)''' \
                % (iptablesCommentPrefix('remote'), container,),
        ]).strip()

    if ip:
        portForward('remove', container, ip, port)
    else:
        sys.stderr.write('- warning: container IP not found, iptables rules were not able to be removed\n')

    if not iptablesOnly:
        try:
            exists = container == subprocess.check_output([
                'bash',
                '-c',
                '''` + bashSafeEnvSetup + `%(lxc)s list --format json | jq -r '.[] | select(.name == "%(container)s").name' '''.strip() \
                    % {'lxc': lxcBin, 'container': container},
            ]).strip()

            if exists:
                # try:
                #     # Stop and destroy the container.
                #     log('stopping container: {}'.format(container))
                #     subprocess.call([lxcBin, 'stop', '--force', container], stdout=sys.stdout, stderr=sys.stderr)
                # except Exception, e:
                #     if not skipStop:
                #         raise e # Otherwise ignore.

                subprocess.call([lxcBin, 'stop', '--force', container])
                subprocess.check_call([
                    'bash',
                    '-c',
                    '''` + bashSafeEnvSetup + `test -z $(zfs mount | awk '{print $1}' | (grep '%(pool)s/%(container)s' || true)) || zfs umount '%(pool)s/%(container)s' '''.strip() \
                        % {'pool': '` + ZFS_CONTAINER_MOUNT + `', 'container': container},
                ])
                retriableCommand(lxcBin, 'delete', '--force', container)
        except subprocess.CalledProcessError as e:
            sys.stderr.write('- warning: deletion of container "%s" failed: %s' % (container, e,))

        try:
            image = '%s%s%s' % (app, dynoDelimiter, version)
            exists = subprocess.check_output([
                'bash',
                '-c',
                '''` + bashSafeEnvSetup + `%(lxc)s image list --format json | jq -r '.[].aliases[] | select(.name == "%(container)s").name' '''.strip() \
                    % {'lxc': lxcBin, 'container': container},
            ]).strip()

            if exists:
                subprocess.check_call(lxcBin, 'image', 'delete', container)
        except subprocess.CalledProcessError as e:
            sys.stderr.write('- warning: deletion of image "%s" failed: %s' % (image, e,))

main(sys.argv)`

var (
	UPSTART        = template.New("UPSTART")
	HAPROXY_CONFIG = template.New("HAPROXY_CONFIG")
	BUILD_PACKS    = map[string]*template.Template{}
)

func (server *Server) initTemplates() error {
	// // Only validate templates if not running in server-mode.
	// if len(os.Args) > 1 && os.Args[1] != "server" {
	// 	return errors.New("initTemplates should only be invoked when running in server-mode")
	// }

	var err error

	UPSTART, err = template.New("UPSTART").Parse(`
# Start on "networking up" state.
# @see http://upstart.ubuntu.com/cookbook/#how-to-establish-a-jobs-start-on-and-stop-on-conditions
start on runlevel [2345]
stop on runlevel [016] or unmounting-filesystem
#exec su ` + DEFAULT_NODE_USERNAME + ` -c "/app/run"
#exec /app/run
pre-start script
    test -d /app/env || mkdir /app/env || true
    touch /app/ip /app/env/PORT || true
    chown ubuntu:ubuntu /app/ip /app/env/PORT || true
    test $(stat -c %U /app/src) = 'root' && chown -R ubuntu:ubuntu /app || true
end script
exec start-stop-daemon --start --chuid ubuntu --exec /bin/sh -- -c "exec envdir /app/env /app/run"
`)
	if err != nil {
		return fmt.Errorf("parsing UPSTART template: %s", err)
	}

	// NB: DefaultSSHHost has `.*@` portion stripped if an `@` symbol is found.
	HAPROXY_CONFIG, err = template.New("HAPROXY_CONFIG").Parse(scripts.HAProxySrc)
	if err != nil {
		return fmt.Errorf("parsing HAPROXY_CONFIG template: %s", err)
	}

	// // Discover all available build-packs.
	// listing, err := ioutil.ReadDir(DIRECTORY + "/build-packs")
	// if err != nil {
	// 	fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
	// 	os.Exit(1)
	// }
	// for _, bp := range server.BuildpacksProvider.Available() {
	// 	if bp.IsDir() {
	// 		log.Infof("Discovered build-pack: %v", bp.Name())
	// 		contents, err := ioutil.ReadFile(DIRECTORY + "/build-packs/" + bp.Name() + "/pre-hook")
	// 		if err != nil {
	// 			fmt.Fprintf(os.Stderr, "fatal: build-pack '%v' missing pre-hook file: %v\n", bp.Name(), err)
	// 			os.Exit(1)
	// 		}
	// 		// Map to template.
	//            tpl, err = template.New("BUILD_" + strings.ToUpper(bp.Name())).Parse(string())
	// 		BUILD_PACKS[bp.Name()] = template.Must(template.New("BUILD_" + strings.ToUpper(bp.Name())).Parse(string(contents)))
	// 	}
	// }

	if server.BuildpacksProvider == nil || len(server.BuildpacksProvider.Available()) == 0 {
		return fmt.Errorf("no build-packs found for provider=%T", server.BuildpacksProvider)
	}

	return nil
}

var (
	// containerCodeTpl is invoked after `git clone` has been run in the container.
	containerCodeTpl = template.Must(template.New("container-code").Parse(`#!/usr/bin/env bash
# set -x

set -o errexit
set -o pipefail
set -o nounset

cd /app/src
git checkout -q -f {{ .Revision }}

# Convert references to submodules to be read-only.
if [[ -f '.gitmodules' ]] ; then 
    echo 'git: converting submodule refs to be read-only'
    sed -i 's,git@github.com:,git://github.com/,g' .gitmodules

    # Update the submodules.
    git submodule init
    git submodule update
else
    echo 'git: project does not appear to have any submodules'
fi

# Clear out and remove all git files from the container; they are unnecessary
# from this point forward.
find . -regex '^.*\.git\(ignore\|modules\|attributes\)?$' -exec rm -rf {} \; 1>/dev/null 2>/dev/null || :
`))

	// preStartTpl is invoked by systemd before the app service is started.
	preStartTpl = template.Must(template.New("container-code").Parse(`#!/usr/bin/env bash
# set -x

set -o errexit
set -o pipefail
set -o nounset

touch /app/ip /app/out /app/env/PORT
chown ` + DEFAULT_NODE_USERNAME + `:` + DEFAULT_NODE_USERNAME + ` /app/ip /app/env/PORT
test "$(stat -c %U /app/src)" = '` + DEFAULT_NODE_USERNAME + `' || chown -R ` + DEFAULT_NODE_USERNAME + `:` + DEFAULT_NODE_USERNAME + ` /app/src
`))

	systemdAppTpl = template.Must(template.New("systemdApp").Parse(`[Unit]
Description=app
After=network.target
# ConditionPathExists=!/etc/ssh/sshd_not_to_be_run

[Service]
Type=simple
ExecStartPre=!/app/preStart.sh
ExecStart=/usr/bin/envdir /app/env /app/run
User=` + DEFAULT_NODE_USERNAME + `
Restart=on-failure

[Install]
WantedBy=multi-user.target`))
)
