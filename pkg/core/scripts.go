package core

import (
	"fmt"
	"text/template"
)

const (
	PRE_RECEIVE = `#!/usr/bin/env bash

set -x

set -o errexit
set -o pipefail
set -o nounset

whoami
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

set -x

set -o errexit
set -o pipefail
set -o nounset

while read oldrev newrev refname; do
    ` + EXE + ` post-receive "$(pwd)" "${oldrev}" "${newrev}" "${refname}"
done`

	LOGIN_SHELL = `#!/usr/bin/env bash
/usr/bin/envdir ` + ENV_DIR + ` /bin/bash`

	// # Cleanup old versions on the shipbuilder build box (only old versions, not the newest/latest version).
	// sudo lxc-ls --fancy | grep --only-matching '^[^ ]\+_v[0-9]\+ *STOPPED' | sed 's/^\([^ ]\+\)\(_v\)\([0-9]\+\) .*/\1 \3 \1\2\3/' | sort -t' ' -k 1,2 -g | awk -F ' ' '$1==app{ printf ",%s", $2 ; next } { app=$1 ; printf "\n%s %s", $1, $2 } END { printf "\n" }' | grep '^[^ ]\+ [0-9]\+,' | sed 's/,[0-9]\+$//' | awk -F ' ' '{ split($2,arr,",") ; for (i in arr) printf "%s_v%s\n", $1, arr[i] }' | xargs -n1 -IX bash -c 'attempts=0; rc=1; while [ $rc -ne 0 ] && [ $attempts -lt 10 ] ; do echo "rc=${rc}, attempts=${attempts} X"; sudo lxc-destroy -n X; rc=$?; attempts=$(($attempts + 1)); done'

	// # Cleanup old zfs container volumes not in use (primarily intended to run on nodes and sb server).
	// containers=$(sudo lxc-ls --fancy | sed "1,2d" | cut -f1 -d" ") ; for x in $(sudo zfs list | sed "1d" | cut -d" " -f1); do if [ "${x}" = "tank" ] || [ "${x}" = "tank/git" ] || [ "${x}" = "tank/lxc" ]; then echo "skipping bare tank, git, or lxc: ${x}"; continue; fi; if [ -n "$(echo $x | grep '@')" ]; then search=$(echo $x | sed "s/^.*@//"); else search=$(echo $x | sed "s/^[^\/]\+\///"); fi; if [ -z "$(echo -e "${containers}" | grep "${search}")" ]; then echo "destroying non-container zfs volume: $x" ; sudo zfs destroy $x; fi; done

	// # Cleanup empty container dirs.
	// for dir in $(find /var/lib/lxc/ -maxdepth 1 -type d | grep '.*_v[0-9]\+_.*_[0-9]\+'); do if test "${dir}" = '.' || test -z "$(echo "${dir}" | sed 's/\/var\/lib\/lxc\///')"; then continue; fi; count=$(find "${dir}/rootfs/" | head -n 3 | wc -l); if test $count -eq 1; then echo $dir $count; echo sudo rm -rf $dir; fi; done

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
preserveVersionsRe=$(
    lxc list --format=json \
    | jq -r '.[] | select(.status == "Stopped") | .name' \
    | grep --only-matching '^[^ ]\+-v[0-9]\+.*$' \
    | sed 's/^\([^ ]\+\)\(-v\)\([0-9]\+\)\(.*\)$/\1 \3 \1\2\3\4/' \
    | sort -t' ' -k 1,2 -g \
    | awk -F ' ' '$1==app{ printf ",%s", $2 ; next } { app=$1 ; printf "\n%s %s", $1, $2 } END { printf "\n" }' \
    | sed 's/\([0-9]\+,\)*\([0-9]\+\)$/\2/' \
    | awk -F ' ' '{ split($2,arr,",") ; for (i in arr) printf "%s-v%s\n", $1, arr[i] }' \
    | uniq \
    | tr '\n' ' ' \
    | sed 's/ /\\|/g' | sed 's/\\|$//' \
)
destroyVersions=$(
    lxc list --format=json \
    | jq -r '.[] | select(.status == "Stopped") | .name' \
    | grep --only-matching '^[^ ]\+-v[0-9]\+.*$' \
    | sed 's/^\([^ ]\+\)\(-v\)\([0-9]\+\)\(.*\)$/\1 \3 \1\2\3\4/' \
    | sort -t' ' -k 1,2 -g \
    | awk -F ' ' '$1==app{ printf ",%s", $2 ; next } { app=$1 ; printf "\n%s %s", $1, $2 } END { printf "\n" }' \
    | grep '^[^ ]\+ [0-9]\+,' \
    | sed 's/,[0-9]\+$//' \
    | awk -F ' ' '{ split($2,arr,",") ; for (i in arr) printf "%s-v%s\n", $1, arr[i] }' \
    | uniq
)

# TODO: Migrate this to ZFS 2.0 / SB LXC ZFS PATH (added 2017-11-04).

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

    lxc delete --force "${name}"
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
    for container in $(lxc list --format=csv | cut -d ',' -f 1) ; do
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

destroyNonContainerVolumes

## Cleanup any empty container directories.
#for dir in $(find /var/lib/lxc/ -maxdepth 1 -type d | grep '[a-zA-Z0-9-]\+_\(v[0-9]\+\(_.\+_[0-9]\+\)\?\|console_[a-zA-Z0-9]\+\)'); do
#    if test "${dir}" = '.' || test -z "$(echo "${dir}" | sed 's/\/var\/lib\/lxc\///')"; then
#        continue
#    fi
#    count=$(find "${dir}/rootfs/" | head -n 3 | wc -l)
#    if test $count -eq 1; then
#        echo $dir $count
#        sudo rm -rf $dir
#    fi
#done

exit $?`
)

var POSTDEPLOY = `#!/usr/bin/python -u
# -*- coding: utf-8 -*-

import os, stat, subprocess, sys, time

defaultLxcFs='` + DefaultLXCFS + `'
lxcDir='` + LXC_DIR + `'
zfsContainerMount='` + ZFS_CONTAINER_MOUNT + `'
container = None
log = lambda message: sys.stdout.write('[{0}] {1}\n'.format(container, message))

def getIp(name):
    with open('` + LXC_DIR + `/' + name + '/rootfs/app/ip') as f:
        return f.read().split('/')[0]

def modifyIpTables(action, chain, ip, port):
    """
    @param action str 'append' or 'delete'.
    @param chain str 'PREROUTING' or 'OUTPUT'.
    """
    global container

    assert action in ('append', 'delete'), 'Invalid action: "{0}", must be "append" or "delete"'
    assert chain in ('PREROUTING', 'OUTPUT'), 'Invalid chain: "{0}", must be "PREROUTING" or "OUTPUT"'.format(chain)
    assert ip is not None and ip != '', 'Invalid ip: "{0}", ip cannot be None or empty'.format(ip)
    assert port is not None and port != '', 'Invalid port: "{0}", port cannot be None or empty'.format(port)

    # Sometimes iptables is being run too many times at once on the same box, and will give an error like:
    #     iptables: Resource temporarily unavailable.
    #     exit status 4
    # We try to detect any such occurrence, and up to N times we'll wait for a moment and retry.
    attempts = 0
    while True:
        child = subprocess.Popen(
            [
                '/sbin/iptables',
                '--table', 'nat',
                '--{0}'.format(action), chain,
                '--proto', 'tcp',
                '--dport', port,
                '--jump', 'DNAT',
                '--to-destination', '{0}:{1}'.format(ip, port),
                '--comment', 'Forward for app-container=%s' % (container,)
            ] + (['--out-interface', 'lo'] if chain == 'OUTPUT' else []),
            stderr=sys.stderr,
            stdout=sys.stdout
        )
        child.communicate()
        exitCode = child.returncode
        if exitCode == 0:
            return
        elif exitCode == 4 and attempts < 40:
            log('iptables: Resource temporarily unavailable (exit status 4), retrying.. ({0} previous attempts)'.format(attempts))
            attempts += 1
            time.sleep(0.5)
            continue
        else:
            raise subprocess.CalledProcessError('iptables failure; exited with status code {0}'.format(exitCode))

def ipsForRulesMatchingPort(chain, port):
    # NB: 'exit 0' added to avoid exit status code 1 when there were no results.
    rawOutput = subprocess.check_output(
        [
            '/sbin/iptables --table nat --list {0} --numeric | grep -E -o "[0-9.]+:{1}" | grep -E -o "^[^:]+"; exit 0' \
                .format(chain, port),
        ],
        shell=True,
        stderr=sys.stderr
    ).strip()
    return rawOutput.split('\n') if len(rawOutput) > 0 else []

def configureIpTablesForwarding(ip, port):
    log('configuring iptables to forward port {0} to {1}'.format(port, ip))
    # Clear out any conflicting pre-existing rules on the same port.
    for chain in ('PREROUTING', 'OUTPUT'):
        conflictingRules = ipsForRulesMatchingPort(chain, port)
        for someOtherIp in conflictingRules:
            modifyIpTables('delete', chain, someOtherIp, port)

    # Add a rule to route <eth0-iface>:<port> TCP packets to the container.
    modifyIpTables('append', 'PREROUTING', ip, port)

    # Add another rule so that the port will be reachable from <eth0-iface>:port from localhost.
    modifyIpTables('append', 'OUTPUT', ip, port)

def cloneContainer(app, container, version, check=True):
    log('cloning container: {0}'.format(container))
    fn = subprocess.check_call if check else subprocess.call
    return fn(
        ['/usr/bin/lxc', 'init', app + '` + DYNO_DELIMITER + `' + version, container],
        stdout=sys.stdout,
        stderr=sys.stderr
    )

def startContainer(container, check=True):
    log('starting container: {}'.format(container))
    fn = subprocess.check_call if check else subprocess.call
    return fn(
        ['/usr/bin/lxc', 'start', container],
        stdout=sys.stdout,
        stderr=sys.stderr
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

def validateMainArgs(argv):
    if len(argv) != 2:
        sys.stderr.write('{} error: missing required argument: container-name\n'.format(sys.argv))
        sys.exit(1)

def parseMainArgs(argv):
    validateMainArgs(argv)
    container = argv[1]
    app, version, process, port = container.rsplit('` + DYNO_DELIMITER + `', 3) # Format is app-version-process-port.
    return (container, app, version, process, port)

def mountContainerFs(container):
    if defaultLxcFs != 'zfs':
        return
    subprocess.check_call(
        ['sudo', 'zfs', 'mount', zfsContainerMount.strip('/') + '/' + container],
        stdout=sys.stdout,
        stderr=sys.stderr
    )

def unmountContainerFs(container):
    if defaultLxcFs != 'zfs':
        return
    subprocess.check_call(
        ['sudo', 'zfs', 'umount', zfsContainerMount.strip('/') + '/' + container],
        stdout=sys.stdout,
        stderr=sys.stderr
    )

def main(argv):
    global container
    #print('main argv={0}'.format(argv))
    if len(argv) > 1 and argv[1] in ('-h', '--help', 'help'):
        showHelpAndExit(argv)

    container, app, version, process, port = parseMainArgs(argv)

    # For safety, even though it's unlikely, try to kill/shutdown any existing container with the same name.
    subprocess.call(['/usr/bin/lxc stop --force {0} 1>&2 2>/dev/null'.format(container)], shell=True)
    subprocess.call(['/usr/bin/lxc delete --force {0} 1>&2 2>/dev/null'.format(container)], shell=True)

    # Clone the specified container.
    cloneContainer(app, container, version)

    ## This line, if present, would prevent the container from booting.
    ##log('scrubbing any "lxc.cap.drop = mac_{0}" lines from container config'.format(container))
    #subprocess.check_call(
    #    ['sed', '-i', '/lxc.cap.drop = mac_{0}/d'.format(container), lxcDir + '/{0}/config'.format(container)],
    #    stdout=sys.stdout,
    #    stderr=sys.stderr
    #)

    log('creating run script for app "{0}" with process type={1}'.format(app, process))
    # NB: The curly braces are kinda crazy here, to get a single '{' or '}' with python.format(), use double curly
    # braces.
    host = '''` + DefaultSSHHost + `'''
    runScript = '''#!/usr/bin/env bash
ip addr show eth0 | grep 'inet.*eth0' | awk '{{print $2}}' > /app/ip
rm -rf /tmp/log
cd /app/src
echo '{port}' > ../env/PORT
while read line || [ -n "$line" ]; do
    process="${{line%%:*}}"
    command="${{line#*: }}"
    if [ "$process" == "{process}" ]; then
        envdir ` + ENV_DIR + ` /bin/bash -c "export PATH=\"$(find /app/.shipbuilder -type d -wholename '*bin' -maxdepth 2):${{PATH}}\"; ( ${{command}} ) 2>&1 | /app/` + BINARY + ` logger --host={host} --app={app} --process={process}.{port}"
    fi
done < Procfile'''.format(port=port, host=host.split('@')[-1], process=process, app=app)
    mountContainerFs(container)
    runScriptFileName = lxcDir + '/{0}/rootfs/app/run'.format(container)
    with open(runScriptFileName, 'w') as fh:
        fh.write(runScript)
    # Chmod to be executable.
    st = os.stat(runScriptFileName)
    os.chmod(runScriptFileName, st.st_mode | stat.S_IEXEC)

    unmountContainerFs(container)
    startContainer(container)

    log('waiting for container to boot and report ip-address')
    numChecks = 45
    # Allow container to bootup.
    ip = None
    for _ in xrange(numChecks):
        time.sleep(1)
        try:
            ip = getIp(container)
            if ip:
                # ip obtained!
                break
        except:
            continue

    if ip:
        log('found ip: {0}'.format(ip))
        configureIpTablesForwarding(ip, port)

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
                    if time.time() - startedTs > maxSeconds: # or attempts > maxAttempts:
                        sys.stderr.write('- error: curl http check failed, {0}\n'.format(e))
                        subprocess.check_call(['/tmp/shutdown_container.py', container, 'destroy-only'])
                        sys.exit(1)
                    else:
                        time.sleep(1)

    else:
        sys.stderr.write('- error: failed to retrieve container ip')
        subprocess.check_call(['/tmp/shutdown_container.py', container, 'destroy-only'])
        sys.exit(1)

main(sys.argv)`

var SHUTDOWN_CONTAINER = `#!/usr/bin/python -u
# -*- coding: utf-8 -*-

import subprocess, sys, time

DefaultLXCFS = '` + DefaultLXCFS + `'
DefaultZFSPool = '` + DefaultZFSPool + `'
container = None
log = lambda message: sys.stdout.write('[{0}] {1}\n'.format(container, message))

def modifyIpTables(action, chain, ip, port):
    """
    @param action str 'append' or 'delete'.
    @param chain str 'PREROUTING' or 'OUTPUT'.
    """
    assert action in ('append', 'delete'), 'Invalid action: "{0}", must be "append" or "delete"'
    assert chain in ('PREROUTING', 'OUTPUT'), 'Invalid chain: "{0}", must be "PREROUTING" or "OUTPUT"'.format(chain)
    assert ip is not None and ip != '', 'Invalid ip: "{0}", ip cannot be None or empty'.format(ip)
    assert port is not None and port != '', 'Invalid port: "{0}", port cannot be None or empty'.format(port)

    # Sometimes iptables is being run too many times at once on the same box, and will give an error like:
    #     iptables: Resource temporarily unavailable.
    #     exit status 4
    # We try to detect any such occurrence, and up to N times we'll wait for a moment and retry.
    attempts = 0
    while True:
        child = subprocess.Popen(
            [
                '/sbin/iptables',
                '--table', 'nat',
                '--{0}'.format(action), chain,
                '--proto', 'tcp',
                '--dport', port,
                '--jump', 'DNAT',
                '--to-destination', '{0}:{1}'.format(ip, port),
            ] + (['--out-interface', 'lo'] if chain == 'OUTPUT' else []),
            stderr=sys.stderr,
            stdout=sys.stdout
        )
        child.communicate()
        exitCode = child.returncode
        if exitCode == 0:
            return
        elif exitCode == 4 and attempts < 15:
            log('iptables: Resource temporarily unavailable (exit status 4), retrying.. ({0} previous attempts)'.format(attempts))
            attempts += 1
            time.sleep(1)
            continue
        else:
            raise subprocess.CalledProcessError('iptables exited with non-zero status code {0}'.format(exitCode))

def ipsForRulesMatchingPort(chain, port):
    # NB: 'exit 0' added to avoid exit status code 1 when there were no results.
    rawOutput = subprocess.check_output(
        [
            '/sbin/iptables --table nat --list {0} --numeric | grep -E --only-matching "[0-9.]+:{1}" | grep -E --only-matching "^[^:]+"; exit 0' \
                .format(chain, port),
        ],
        shell=True,
        stderr=sys.stderr
    ).strip()
    return rawOutput.split('\n') if len(rawOutput) > 0 else []

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

def showHelpAndExit(argv):
    message = '''usage: {} [container-name] ["destroy-only"?]
       where "container-name" is of the form: [app]_[version]_[process]_[port]

       "destroy-only" will only go about destroying the container (not shutting it down).

       For example, here is how you would terminate a container with the following attributes:

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

def validateMainArgs(argv):
    if len(argv) != 2 or (len(argv) > 2 and argv[2] == 'destroy-only'):
        sys.stderr.write('{} error: invalid arguments, see [-h|--help] for usage details.\n'.format(sys.argv))
        sys.exit(1)

def parseMainArgs(argv):
    validateMainArgs(argv)
    container = argv[1]
    app, version, process, port = container.rsplit('` + DYNO_DELIMITER + `', 3) # Format is app-version-process-port.
    return (container, app, version, process, port)

def main(argv):
    global container
    #print('main argv={0}'.format(argv))
    if len(argv) > 1 and argv[1] in ('-h', '--help', 'help'):
        showHelpAndExit(argv)

    container, app, version, process, port = parseMainArgs(argv)


def main(argv):
    global container
    container = argv[1]
    destroyOnly = len(argv) > 2 and argv[2] == 'destroy-only'
    port = container.split('` + DYNO_DELIMITER + `').pop()

    try:
        # Stop and destroy the container.
        log('stopping container: {}'.format(container))
        subprocess.check_call(['/usr/bin/lxc', 'stop', '--force', container], stdout=sys.stdout, stderr=sys.stderr)
    except Exception, e:
        if not destroyOnly:
            raise e # Otherwise ignore.

    if DefaultLXCFS == 'zfs':
        try:
            retriableCommand('/sbin/zfs', 'destroy', '-r', DefaultZFSPool + '/' + container)
        except subprocess.CalledProcessError, e:
            print('warn: zfs destroy command failed: {0}'.format(e))

    retriableCommand('/usr/bin/lxc', 'delete', '--force', container)

    for chain in ('PREROUTING', 'OUTPUT'):
        rules = ipsForRulesMatchingPort(chain, port)
        for ip in rules:
            log('removing iptables {0} chain rule: port={1} ip={2}'.format(chain, port, ip))
            modifyIpTables('delete', chain, ip, port)

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
	HAPROXY_CONFIG, err = template.New("HAPROXY_CONFIG").Parse(`
global
    maxconn 32000
    # NB: Base HAProxy logging configuration is as per: http://kvz.io/blog/2010/08/11/haproxy-logging/
    #log 127.0.0.1 local1 info
    log {{.LogServerIpAndPort}} local1 info
    chroot /var/lib/haproxy
    stats socket /run/haproxy/admin.sock mode 660 level admin
    stats timeout 30s
    user haproxy
    group haproxy
    daemon

    # Default SSL material locations
    ca-base /etc/ssl/certs
    crt-base /etc/ssl/private

    # Default ciphers to use on SSL-enabled listening sockets.
    # For more information, see ciphers(1SSL).
    ssl-default-bind-ciphers kEECDH+aRSA+AES:kRSA+AES:+AES256:RC4-SHA:!kEDH:!LOW:!EXP:!MD5:!aNULL:!eNULL

defaults
    log global
    mode http
    option tcplog
    retries 4
    option redispatch
    timeout connect 5000
    timeout client 30000
    timeout server 30000
    #option http-server-close

frontend frontend
    bind 0.0.0.0:80
    # Require SSL
    redirect scheme https code 301 if !{ ssl_fc }
    bind 0.0.0.0:443 ssl crt /etc/haproxy/certs.d no-sslv3
    maxconn 32000
    option httplog
    option http-pretend-keepalive
    option forwardfor
    option http-server-close
{{range $app := .Applications}}
    {{if .Domains}}use_backend {{$app.Name}}{{if $app.Maintenance}}-maintenance{{end}} if { {{range .Domains}} hdr(host) -i {{.}} {{end}} }{{end}}
{{end}}
    {{if and .HaProxyStatsEnabled .HaProxyCredentials .LoadBalancers}}use_backend load_balancer if { {{range .LoadBalancers }} hdr(host) -i {{.}} {{end}} }{{end}}

{{with $context := .}}{{range $app := .Applications}}
backend {{.Name}}
    balance roundrobin
    reqadd X-Forwarded-Proto:\ https if { ssl_fc }
    option forwardfor
    option abortonclose
    option httpchk GET / HTTP/1.1\r\nHost:\ {{.FirstDomain}}
  {{range $app.Servers}}
    server {{.Host}}-{{.Port}} {{.Host}}:{{.Port}} check port {{.Port}} observe layer7
  {{end}}{{if and $context.HaProxyStatsEnabled $context.HaProxyCredentials}}
    stats enable
    stats uri /haproxy
    stats auth {{$context.HaProxyCredentials}}
  {{end}}
{{end}}{{end}}

{{range .Applications}}
backend {{.Name}}-maintenance
    acl static_file path_end .gif || path_end .jpg || path_end .jpeg || path_end .png || path_end .css
    reqirep ^GET\ (.*)                    GET\ {{.MaintenancePageBasePath}}\1     if static_file
    reqirep ^([^\ ]*)\ [^\ ]*\ (.*)       \1\ {{.MaintenancePageFullPath}}\ \2    if !static_file
    reqirep ^Host:\ .*                    Host:\ {{.MaintenancePageDomain}}
    reqadd Cache-Control:\ no-cache,\ no-store,\ must-revalidate
    reqadd Pragma:\ no-cache
    reqadd Expires:\ 0
    rspirep ^HTTP/([^0-9\.]+)\ 200\ OK    HTTP/\1\ 503\ 
    rspadd Retry-After:\ 60
    server s3 {{.MaintenancePageDomain}}:80
{{end}}

{{if and .HaProxyStatsEnabled .HaProxyCredentials .LoadBalancers}}
backend load_balancer
    stats enable
    stats uri /haproxy
    stats auth {{.HaProxyCredentials}}
{{end}}
`)
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
set -x

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
set -x

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
