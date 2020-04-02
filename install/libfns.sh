set -o nounset

if [[ "$(echo "${SB_DEBUG:-}" | tr '[:upper:]' '[:lower:]')" =~ ^1|true|t|yes|y$ ]] ; then
    echo 'INFO: debug mode enabled'
    export SB_DEBUG_BASH='set -x'
else
    export SB_DEBUG_BASH=':'
fi

export SB_REPO_PATH="${GOPATH:-${HOME}/go}/src/github.com/jaytaylor/shipbuilder"
export SB_SSH='ssh -o BatchMode=yes -o StrictHostKeyChecking=no'

export goVersion='1.14'
export lxcBaseImage='ubuntu:16.04'

# Content of this variable is invoked during funtion RETURN traps.
export __restorestates_trap='IFS=$'"'"'\n'"'"' ; for ss in $(echo -e "${setstates:-}" | sort --reverse -k3) ; do eval ${ss} 1>/dev/null 2>/dev/null ; done'

# function backtrace () {
#     local deptn=${#FUNCNAME[@]}

#     for ((i=1; i<$deptn; i++)); do
#         local func="${FUNCNAME[$i]}"
#         local line="${BASH_LINENO[$((i-1))]}"
#         local src="${BASH_SOURCE[$((i-1))]}"
#         printf '%*s' $i '' # indent
#         echo "at: $func(), $src, line $line"
#     done
# }
# function trace_top_caller () {
#     local func="${FUNCNAME[1]}"
#     local line="${BASH_LINENO[0]}"
#     local src="${BASH_SOURCE[0]}"
#     echo "  called from: $func(), $src, line $line"
# }
# set -o errtrace
# trap 'trace_top_caller' ERR

function checkUserPermissions() {
    if [ "$(id -u)" -eq 0 ] ; then
        echo 'error: must not be run as root' 1>&2
        exit 1
    fi
    sudo -n bash -c ':'
    if [ $? -ne 0 ] ; then
        echo 'error: must be run by a user who has passwordless sudo access'
        exit 1
    fi
}

function abortIfNonZero() {
    # @param $1 command return code/exit status (e.g. $?, '0', '1').
    # @param $2 error message if exit status was non-zero.
    local rc=${1:-}
    local what=${2:-}
    test ${rc} -ne 0 && echo "error: ${what} exited with non-zero status ${rc}" 1>&2 && exit ${rc} || :
}

function abortWithError() {
    echo "${1:-}" 1>&2 && exit 1
}

function warnIfNonZero() {
    # @param $1 command return code/exit status (e.g. $?, '0', '1').
    # @param $2 error message if exit status was non-zero.
    local rc=${1:-}
    local what=${2:-}
    test ${rc} -ne 0 && echo "warn: ${what} exited with non-zero status ${rc}" 1>&2 || :
}

function autoDetectServer() {
    # Attempts to auto-detect the server host by reading the contents of ../env/SB_SSH_HOST.
    if [ -r "$(dirname "$0")/../env/SB_SSH_HOST" ] ; then
        export SB_SSH_HOST="$(head -n 1 "$(dirname "$0")/../env/SB_SSH_HOST")"
        test -z "${SB_SSH_HOST}" && echo 'error: autoDetectServer(): lxc filesystem auto-detection failed: ../env/SB_SSH_HOST file empty?' 1>&2 && exit 1 || :
        echo "info: auto-detected shipbuilder host: ${SB_SSH_HOST}"
    else
        echo 'error: server auto-detection failed: no such file: ../env/SB_SSH_HOST' 1>&2 && exit 1 || :
    fi
}

function autoDetectFilesystem() {
    # Attempts to auto-detect the target filesystem type by reading the contents of ../env/LXC_FS.
    if [ -r "$(dirname "$0")/../env/SB_LXC_FS" ] ; then
        export SB_LXC_FS="$(head -n 1 "$(dirname "$0")/../env/SB_LXC_FS")"
        test -z "${SB_LXC_FS}" && echo 'error: autoDetectFilesystem(): lxc filesystem auto-detection failed: ../env/SB_LXC_FS file empty?' 1>&2 && exit 1 || :
        echo "info: auto-detected lxc filesystem: ${SB_LXC_FS}"
    else
        echo 'error: autoDetectFilesystem(): lxc filesystem auto-detection failed: no such file: ../env/SB_LXC_FS' 1>&2 && exit 1 || :
    fi
}

function autoDetectZfsPool() {
    # When fs type is 'zfs', attempt to auto-detect the zfs pool name to create by reading the contents of ../env/ZFS_POOL.
    test -z "${SB_LXC_FS}" && autoDetectFilesystem # Attempt to ensure that the target filesystem type is available.
    if [ "${SB_LXC_FS}" = 'zfs' ] ; then
        if [ -r "$(dirname "$0")/../env/SB_ZFS_POOL" ] ; then
            export SB_ZFS_POOL="$(head -n 1 "$(dirname "$0")/../env/SB_ZFS_POOL")"
            test -z "${SB_ZFS_POOL}" && echo 'error: autoDetectZfsPool(): zfs pool auto-detection failed: ../env/SB_ZFS_POOL file empty?' 1>&2 && exit 1 || :
            echo "info: auto-detected zfs pool: ${SB_ZFS_POOL}"
            # Validate to ensure zfs pool name won't conflict with typical ubuntu root-fs items.
            for x in bin boot dev etc git home lib lib64 media mnt opt proc root run sbin selinux srv sys tmp usr var vmlinuz zfs-kstat ; do
                test "${SB_ZFS_POOL}" = "${x}" && echo "error: invalid zfs pool name detected, '${x}' is a forbidden because it may conflict with a system directory" 1>&2 && exit 1 || :
            done
        else
            echo 'error: autoDetectZfsPool(): zfs pool auto-detection failed: no such file: ../env/SB_ZFS_POOL' 1>&2 && exit 1 || :
        fi
    fi
}

function autoDetectVars() {
    test -n "${SB_SSH_HOST:-}" || autoDetectServer
    test -n "${SB_LXC_FS:-}" || autoDetectFilesystem
    test -n "${SB_ZFS_POOL:-}" || autoDetectZfsPool
}

# dumpAutoDetectedVars is useful to pass auto-detected variables explicitly to a
# command which won't be able to do the auto-detection itself.
#
# For example, when libfns.sh has been rsync'd to a remote machine and a
# function with a dependency on one or more auto-detected variables needs to be
# invoked through SSH.
#
# Dumps out variables contained in the autoDetectVars function in the form of:
#
#     k1=v1 k2=v2 .. kN=vN
#
function dumpAutoDetectedVars() {
    autoDetectVars

    declare -f autoDetectVars | grep --only-matching 'SB_[A-Z0-9_]\+' | xargs -n1 -IX bash -c 'echo X=${X}' | tr $'\n' ' '
}

function verifySshAndSudoForHosts() {
    # @param $1 string. List of space-delimited SSH connection strings.
    local sshHosts="${1:-}"

    local result
    local rc

    echo "info: verifying ssh and sudo access for $(echo "${sshHosts}" | tr ' ' '\n' | grep -v '^ *$' | wc -l | sed 's/^[ \t]*//g') hosts"
    for sshHost in ${sshHosts} ; do
        echo -n "info:     testing host ${sshHost} .. "
        result=$(ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' -o 'ConnectTimeout=15' -q "${sshHost}" "sudo -n echo 'succeeded' 2>/dev/null")
        rc=$?
        test ${rc} -ne 0 && echo 'failed' && abortWithError "error: ssh connection test failed for host: ${sshHost} (exited with status code: ${rc})"
        test -z "${result}" && echo 'failed' && abortWithError "error: sudo access test failed for host: ${sshHost}"
        echo 'succeeded'
    done
}

function initSbServerKeys() {
    # @precondition $SB_SSH_HOST must not be empty.
    test -z "${SB_SSH_HOST:-}" && echo 'error: initSbServerKeys(): required parameter $SB_SSH_HOST cannot be empty' 1>&2 && exit 1

    echo "info: checking SB server=${SB_SSH_HOST} SSH keys, will generate if missing"

    ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' ${SB_SSH_HOST} '/bin/bash -c '"'"'
    echo "remote: info: setting up pub/private SSH keys so that root and main users can SSH in to either account"
    function abortIfNonZero() {
        local rc=${1:-}
        local what=${2:-}
        test ${rc} -ne 0 && echo "remote: error: ${what} exited with non-zero status ${rc}" && exit ${rc} || :
    }
    if ! [ -e ~/.ssh/id_rsa.pub ] ; then
        echo "remote: info: generating a new private/public key-pair for main user"
        rm -f ~/.ssh/id_*
        abortIfNonZero $? "removing old keys failed"
        ssh-keygen -f ~/.ssh/id_rsa -t rsa -N ""
        abortIfNonZero $? "ssh-keygen command failed"
    fi

    if ! [ -e ~/.ssh ] ; then
        mkdir ~/.ssh
        chmod 700 ~/.ssh
    fi
    if ! [ -e ~/.ssh/authorized_keys ] || [ -z "$(grep "$(cat ~/.ssh/id_rsa.pub)" ~/.ssh/authorized_keys)" ] ; then
        echo "remote: info: adding main user to main user authorized_keys"
        cat ~/.ssh/id_rsa.pub >> ~/.ssh/authorized_keys
        abortIfNonZero $? "appending public-key to authorized_keys command"
        chmod 600 ~/.ssh/authorized_keys
        abortIfNonZero $? "chmod 600 ~/.ssh/authorized_keys command"
    fi

    if ! sudo -n test -e /root/.ssh ; then
        sudo -n mkdir /root/.ssh
        sudo -n chmod 700 ~/.ssh
    fi
    if ! sudo -n test -e /root/.ssh/authorized_keys || sudo -n test -z "$(sudo -n grep "$(cat ~/.ssh/id_rsa.pub)" /root/.ssh/authorized_keys)" ; then
        echo "remote: info: adding main user to root user authorized_keys"
        cat ~/.ssh/id_rsa.pub | sudo -n tee -a /root/.ssh/authorized_keys >/dev/null
        abortIfNonZero $? "appending public-key to authorized_keys command"
        sudo -n chmod 600 /root/.ssh/authorized_keys
        abortIfNonZero $? "chmod 600 /root/.ssh/authorized_keys command"
    fi

    if ! sudo -n test -e /root/.ssh/id_rsa.pub ; then
        echo "remote: info: generating a new private/public key-pair for root user"
        if [ -n "$(sudo -n bash -c "/root/.ssh/id_*")" ] ; then
            backupDir="sb_backup_$(date +%s)"
            sudo -n mkdir "/root/.ssh/${backupDir}"
            sudo -n bash -c "mv /root/.ssh/id_* /root/.ssh/${backupDir}/"
            abortIfNonZero $? "backing up old keys failed"
        fi
        sudo -n ssh-keygen -f /root/.ssh/id_rsa -t rsa -N ""
        abortIfNonZero $? "ssh-keygen command failed"
    fi
    if [ -z "$(sudo -n grep "$(sudo -n cat /root/.ssh/id_rsa.pub)" /root/.ssh/authorized_keys)" ] ; then
        echo "remote: info: adding root to root user authorized_keys"
        sudo -n cat /root/.ssh/id_rsa.pub | sudo -n tee -a /root/.ssh/authorized_keys >/dev/null
        abortIfNonZero $? "appending public-key to authorized_keys command"
        sudo -n chmod 600 /root/.ssh/authorized_keys
        abortIfNonZero $? "chmod 600 /root/.ssh/authorized_keys command"
    fi
    if [ -z "$(grep "$(sudo -n cat /root/.ssh/id_rsa.pub)" ~/.ssh/authorized_keys)" ] ; then
        echo "remote: info: adding root to main user authorized_keys"
        sudo -n cat /root/.ssh/id_rsa.pub >> ~/.ssh/authorized_keys
        abortIfNonZero $? "appending public-key to authorized_keys command"
        sudo -n chmod 600 ~/.ssh/authorized_keys
        abortIfNonZero $? "chmod 600 ~/.ssh/authorized_keys command"
    fi'"'"
    abortIfNonZero $? 'ssh key initialization'
    echo 'info: ssh key initialization succeeded'
}

function getSbServerPublicKeys() {
    local sshHost=${1:-}

    test -z "${sshHost}" && echo 'error: getSbServerPublicKeys(): missing required parameter: SSH hostname' 1>&2 && exit 1

    local pubKeys

    initSbServerKeys

    echo "info: retrieving public-keys from shipbuilder server: ${sshHost}"

    ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' ${sshHost} "test -f ~/.ssh/id_rsa.pub || ssh-keygen -f ~/.ssh/id_rsa -t rsa -N \"\" && sudo -n bash -c 'set -o errexit && test -f /root/.ssh/id_rsa.pub || ssh-keygen -f /root/.ssh/id_rsa -t rsa -N \"\"'"
    abortIfNonZero $? "Ensuring SSH public-keys exist on ssh-host=${sshHost}"

    pubKeys=$(ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' ${sshHost} 'cat ~/.ssh/id_rsa.pub && echo "." && sudo -n cat /root/.ssh/id_rsa.pub')
    abortIfNonZero $? "SSH public-key retrieval failed for ssh-host=${sshHost}"

    export SB_UNPRIVILEGED_PUBKEY=$(echo "${pubKeys}" | grep --before 100 '^\.$' | grep -v '^\.$')
    export SB_ROOT_PUBKEY=$(echo "${pubKeys}" | grep --after 100 '^\.$' | grep -v '^\.$')

    if [ -z "${SB_UNPRIVILEGED_PUBKEY}" ] ; then
        echo 'error: failed to obtain build-server public-key for unprivileged user' 1>&2
        exit 1
    fi
    echo "info: obtained unprivileged public-key: ${SB_UNPRIVILEGED_PUBKEY}"
    if [ -z "${SB_ROOT_PUBKEY}" ] ; then
        echo 'error: failed to obtain build-server public-key for root user' 1>&2
        exit 1
    fi
    echo "info: obtained root public-key: ${SB_ROOT_PUBKEY}"
}

function installAccessForSshHost() {
    # @precondition Variable $sshKeysCommand must be initialized and not empty.
    # @param $1 SSH connection string (e.g. user@host)
    local sshHost=${1:-}

    local remoteCommand

    test -z "${sshHost}" && echo 'error: installAccessForSshHost(): missing required parameter: SSH hostname' 1>&2 && exit 1

    if [ -z "${SB_UNPRIVILEGED_PUBKEY:-}" ] || [ -z "${SB_ROOT_PUBKEY:-}" ] ; then
        getSbServerPublicKeys ${SB_SSH_HOST}
    fi

    test -z "${sshHost}" && echo 'error: installAccessForSshHost(): missing required parameter: SSH hostname' 1>&2 && exit 1 || :

    echo "info: setting up remote access from build-server to host: ${sshHost}"

    remoteCommand='/bin/bash -c '"'"'
    '"${SB_DEBUG_BASH}"'

    set -o errexit
    set -o pipefail
    set -o nounset

    echo "remote: checking keys for unprivileged user.."
    if [ -z "$(grep "'"${SB_UNPRIVILEGED_PUBKEY}"'" ~/.ssh/authorized_keys)" ]; then
        echo "'"${SB_UNPRIVILEGED_PUBKEY}"'" | tee -a ~/.ssh/authorized_keys >/dev/null
    fi
    if [ -z "$(sudo -n grep "'"${SB_ROOT_PUBKEY}"'" ~/.ssh/authorized_keys)" ] ; then
        echo "'"${SB_ROOT_PUBKEY}"'" | tee -a ~/.ssh/authorized_keys >/dev/null
    fi

    echo "remote: checking keys for root user.."
    if [ -z "$(sudo -n grep "'"${SB_UNPRIVILEGED_PUBKEY}"'" /root/.ssh/authorized_keys)" ] ; then
         echo "'"${SB_UNPRIVILEGED_PUBKEY}"'" | sudo -n tee -a /root/.ssh/authorized_keys >/dev/null
    fi
    if [ -z "$(sudo -n grep "'"${SB_ROOT_PUBKEY}"'" /root/.ssh/authorized_keys)" ] ; then
        echo "'"${SB_ROOT_PUBKEY}"'" | sudo -n tee -a /root/.ssh/authorized_keys >/dev/null
    fi

    chmod 600 ~/.ssh/authorized_keys
    sudo -n chmod 600 /root/.ssh/authorized_keys

    exit 0
    '"'"

    echo "remoteCommand=${remoteCommand}"

    ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' ${sshHost} "${remoteCommand}"
    abortIfNonZero $? "ssh access installation failed for host ${sshHost}"

    echo 'info: ssh access installation succeeded'
}

die() {
    echo "ERROR: $*" 1>&2
    exit 1
}

function installLxc() {
    # @param $1 $lxcFs lxc filesystem to use (zfs, btrfs are both supported).
    # @param $2 $zfsPoolArg ZFS pool name.
    local lxcFs="${1:-}"
    local zfsPoolArg="${2:-}"
    local device="${3:-}"

    test -z "${lxcFs}" && echo 'error: installLxc() missing required parameter: $lxcFs' 1>&2 && exit 1 || :
    test -z "${zfsPoolArg}" && echo 'error: installLxc() missing required parameter: $zfsPoolArg' 1>&2 && exit 1 || :

    local rc
    local fsPackages
    local required
    local recommended

    sudo -n apt update
    abortIfNonZero $? "command 'apt update'"

    # Legacy migration: zfs-fuse dependency is now switched to zfsutils-linux
    # for shipbuilder v2.
    # LXC and LXD get installed via snap.
    sudo -n apt remove --yes --purge zfs-fuse lxd lxd-client lxc lxc1 lxc2 liblxc1 lxc-common lxcfs
    abortIfNonZero $? "command 'apt remove --yes --purge zfs-fuse lxd lxd-client lxc lxc1 lxc2 liblxc1 lxc-common lxcfs'"

    # <pre-cleanup>
    sudo -n systemctl stop snap.lxd.daemon
    #abortIfNonZero $? "systemctl stop snap.lxd.daemon"

        # Add supporting package(s) for selected filesystem type.
    fsPackages="$(test "${lxcFs}" = 'btrfs' && echo 'btrfs-tools' || :) $(test "${lxcFs}" = 'zfs' && echo 'zfsutils-linux' || :)"

    required="${fsPackages} git mercurial bzr build-essential bzip2 daemontools ntp ntpdate jq"
    echo "info: installing required build-server packages: ${required}"
    sudo -n apt install --yes ${required}
    abortIfNonZero $? "command 'apt install --yes ${required}'"

    recommended='htop iotop unzip screen bzip2 bmon iptraf-ng'
    echo "info: installing recommended packages: ${recommended}"
    sudo -n apt install --yes ${recommended}
    abortIfNonZero $? "command 'apt install --yes ${recommended}'"

    sudo -n rm -rf /var/lib/lxd
    abortIfNonZero $? "command: 'rm -rf /var/lib/lxd'"

    sudo -n ln -s /var/snap/lxd/common/lxd /var/lib/lxd
    abortIfNonZero $? "command: 'ln -s /var/snap/lxd/common/lxd /var/lib/lxd'"
    # </pre-cleanup>

    echo 'info: supported versions of lxc+lxd must be installed'
    echo 'info: as of 2017-12-27, ubuntu comes with lxc+lxd=v2.0.11 by default, and we require lxc=v2.1.1 lxd=2.2.1 or newer'
    if ! grep -q '^lxd:' /etc/group ; then
        sudo -n groupadd --system lxd
        rc=$?
        # NB: if group already exists, groupadd exits with status code 9.
        if [ ${rc} -ne 0 ] && [ ${rc} -ne 9 ] ; then
            abortWithError "command 'groupadd --system lxd' exited with unhappy non-zero status code ${rc}"
        fi
    fi

    if ! getent group lxd | grep -q '\broot\b' ; then
        sudo -n usermod -G lxd -a root
        abortIfNonZero $? "command 'usermod -G lxd -a root'"
    fi

    echo 'info: installing lxd via snap'

    set -o errexit

    if [ -n "$(snap list | awk '/lxd/ { print }')" ]; then
        echo 'INFO: LXD snap installation detected' 1>&2
        echo 'info: skipping lxd installation, already appears to be installed'
        return
#        if ! sudo -n snap remove lxd --purge; then
#            # n.b. Sometimes this resolves "device busy" errors due to tank being mounted
#            #      under /var/lib/lxd/.
#            sudo -n zpool destroy -f tank
#            sudo -n snap remove lxd --purge
#        fi
    fi
#
#    if [ -n "$(sudo -n zpool list | awk '/tank/ { print }')" ]; then
#        sudo -n zpool destroy -f tank
#    fi

    cd /tmp

    #
    # Generated via:
    #     snap download lxd --channel=3.0/stable
    #     snap ack lxd_11348.assert
    #
    curl -sSLO 'https://whaleymood.gigawatt.io/ts/lxd_11348.assert'
    curl -sSLO 'https://whaleymood.gigawatt.io/ts/lxd_11348.snap'
    curl -sSLO 'https://whaleymood.gigawatt.io/ts/SHA-256'

    sha256sum --check SHA-256

    sudo -n snap ack 'lxd_11348.assert'
    sudo -n snap install 'lxd_11348.snap'

    cd - 2>/dev/null

    sudo -n lxd init --preseed << EOF
config: {}
networks:
- config:
    ipv4.address: auto
    ipv6.address: auto
  description: ""
  managed: false
  name: lxdbr0
  type: ""
storage_pools:
- config:
    source: ${device}
  description: ""
  name: ${zfsPoolArg}
  driver: zfs
profiles:
- config: {}
  description: ""
  devices:
    eth0:
      name: eth0
      nictype: bridged
      parent: lxdbr0
      type: nic
    root:
      path: /
      pool: ${zfsPoolArg}
      type: disk
  name: default
cluster: null
EOF

    # Create zfs storage tank only if not already present.
    if ! sudo -n lxc storage list | grep -q "${zfsPoolArg}" ; then
        sudo -n lxc storage create "${zfsPoolArg}" zfs "source=${device}"
    fi

    set +o errexit

    echo "info: installed version of lxc=$(sudo -n lxc version) and lxd=$(sudo -n lxd --version) (all must be v2.21 or newer)"
    echo 'info: installLxc() succeeded'
}

function setupSysctlAndLimits() {
    local fsValue='1048576'
    local limitsValue='100000'

    echo 'info: installing config params to /etc/sysctl.conf'
    for param in max_queued_events max_user_instances max_user_watches ; do
        sudo -n sed -i "/^fs\.inotify\.${param} *=.*\$/d" /etc/sysctl.conf
        abortIfNonZero $? "cleaning config param=${param} from /etc/sysctl.conf"
        echo "fs.inotify.${param} = ${fsValue}" | sudo -n tee -a /etc/sysctl.conf
        abortIfNonZero $? "setting config param=${param} in /etc/sysctl.conf"
    done

    echo 'info: installing config params to /etc/security/limits.conf'
    for param in soft hard ; do
        sudo -n sed -i "/^\* ${param} .*\$/d" /etc/security/limits.conf
        abortIfNonZero $? "cleaning config param=${param} from /etc/security/limits.conf"
        echo "* ${param} nofile ${limitsValue}" | sudo -n tee -a /etc/security/limits.conf
        abortIfNonZero $? "setting config param=${param} in /etc/security/limits.conf"
    done
}

function prepareZfsDirs() {
    # local mvPath

    # Create lxc and git volumes and set mountpoints.
    for volume in git ; do
        #test -z "$(sudo -n zfs list -o name | sed '1d' | grep "^${zfsPoolArg}\/${volume}")" && sudo -n zfs create -o compression=on "${zfsPoolArg}/${volume}" || :
        test -n "$(sudo -n zfs list -o name | sed '1d' | grep "^${zfsPoolArg}\/${volume}")" || sudo -n zfs create -o compression=on "${zfsPoolArg}/${volume}"
        abortIfNonZero $? "command 'zfs create -o compression=on ${zfsPoolArg}/${volume}'"

        sudo -n zfs set "mountpoint=/${volume}" "${zfsPoolArg}/${volume}"
        abortIfNonZero $? "setting mountpoint via 'zfs set mountpoint=/${zfsPoolArg}/${volume} ${zfsPoolArg}/${volume}'"

        sudo -n zfs umount "${zfsPoolArg}/${volume}" 2>/dev/null || :

        sudo -n zfs mount "${zfsPoolArg}/${volume}"
        abortIfNonZero $? "zfs mount'ing ${zfsPoolArg}/${volume}"
    done

    # Mount remaining volumes under LXC base path (rather than $zfsPoolArg
    # [e.g. "/tank"]).
    lxcBasePath=/var/lib/lxd

    # if [ -h "${lxcBasePath}" ] ; then
    #     sudo -n unlink "${lxcBasePath}"
    #     abortIfNonZero $? "command 'unlink ${lxcBasePath}'"
    # elif [ -d "${lxcBasePath}" ] ; then
    #     mvPath="${lxcBasePath}-$(date +%Y%m%d)"
    #     if [ -e "${mvPath}" ] ; then
    #         abortWithError "Refusing to rename ${lxcBasePath} to ${mvPath} because ${mvPath} already exists"
    #     fi
    #     sudo -n mv "${lxcBasePath}" "${mvPath}"
    #     abortIfNonZero $? "command 'rmdir ${lxcBasePath}'"
    # fi
    # sudo -n ln -s /var/snap/lxd/common/ "${lxcBasePath}"
    # abortIfNonZero $? "symlinking /var/snap/lxd/common to ${lxcBasePath}"

    for volume in containers images snapshots ; do
        sudo -n zfs destroy -r "${zfsPoolArg}/${volume}" 2>/dev/null || :

#        test -n "$(sudo -n zfs list -o name | sed '1d' | grep "^${zfsPoolArg}\/${volume}")" || sudo -n zfs create -o compression=on "${zfsPoolArg}/${volume}"
#        abortIfNonZero $? "command 'zfs create -o compression=on ${zfsPoolArg}/${volume}'"
#
#        if [ -z "$(sudo -n zfs list -o mountpoint | grep "^${lxcBasePath}\/${volume}")" ]; then
#            sudo -n zfs set "mountpoint=${lxcBasePath}/${volume}" "${zfsPoolArg}/${volume}"
#            abortIfNonZero $? "setting mountpoint via 'zfs set mountpoint=${lxcBasePath}/${volume} ${zfsPoolArg}/${volume}'"
#        fi
#
#        sudo -n zfs umount "${zfsPoolArg}/${volume}" 2>/dev/null || :
#
#        sudo -n zfs mount "${zfsPoolArg}/${volume}"
#        abortIfNonZero $? "zfs mount'ing ${zfsPoolArg}/${volume}"
#
#        # sudo -n unlink "/${volume}" 2>/dev/null || :
#
#        # sudo -n ln -s "/${zfsPoolArg}/${volume}" "/${volume}"
#        # abortIfNonZero $? "setting up symlink for volume=${volume}"
    done
}

function configureLxdNetworking() {
    local lxcNetExistsTest
    local topInterface

    # Setup LXC/LXD networking.
    ip addr show lxdbr0 1>/dev/null 2>/dev/null
    if [ $? -eq 0 ] ; then
        test -n "$(ip addr show lxdbr0 | grep ' inet ')" || sudo -n lxc network delete lxdbr0
        abortIfNonZero $? "lxc/lxd removal of non-ipv4 network bridge lxdbr0 (recommendation: reboot and re-run installer)"
    fi

    lxcNetExistsTest="$(sudo -n lxc network show lxdbr0 2>/dev/null)"
    if [ -z "${lxcNetExistsTest}" ] ; then
        sudo -n lxc network create lxdbr0 ipv6.address=none ipv4.address=10.0.1.1/24 ipv4.nat=true
        abortIfNonZero $? "lxc/lxd ipv4 network bridge creation of lxdbr0"
    fi

    topInterface=$(ip addr | grep '^[0-9]\+: \([^l]\|l[^o]\)' | head -n 1 | awk '{print $2}' | tr -d ':')
    if [ -z "${topInterface}" ] ; then
        abortWithError "no network interface found to attach to LXC"
    fi

    sudo -n lxc network detach-profile lxdbr0 default ${topInterface} 2>/dev/null || :

    sudo -n lxc network attach-profile lxdbr0 default ${topInterface}
    abortIfNonZero $? "command 'lxc network attach-profile lxdbr0 default ${topInterface}'"
}

function configureLxdZfs() {
    storage="$(sudo -n lxc storage show "${zfsPoolArg}" 2>/dev/null)"
    if [ -z "${storage}" ] ; then
        sudo -n lxc storage create "${zfsPoolArg}" zfs "source=${device}"
        abortIfNonZero $? "command 'lxc storage create ${zfsPoolArg} zfs source=${device}'"
    fi

    if [ -z "$(sudo -n lxc profile device show default | grep -A3 '^root:' | grep "pool: ${zfsPoolArg}")" ] ; then
        sudo -n lxc profile device remove default root

        sudo -n lxc profile device add default root disk path=/ "pool=${zfsPoolArg}"
        abortIfNonZero $? "LXC root zfs device assertion"
    fi
    # sudo -n lxc profile device show default || sudo -n lxc profile device add default root disk path=/ "pool=${zfsPoolArg}"
    # abortIfNonZero $? "LXC root zfs device assertion"
}

function configureLxd() {
    # @precondition $SB_SSH_HOST must not be empty.
    test -z "${SB_SSH_HOST:-}" && echo 'error: configureLxd(): required parameter $SB_SSH_HOST cannot be empty' 1>&2 && exit 1

    local lxcFs=${1:-}
    local isServer=${2:-}

    test -z "${lxcFs}" && echo 'configureLxd: missing required parameter: lxcFs' 1>&2 && exit 1 || :

    local sbServerRemote

    sudo -n systemctl restart snap.lxd.daemon
    abortIfNonZero $? "command 'systemctl restart snap.lxd.daemon'"

    # Give the LXD daemon a moment to come up.
    sleep 3

    sudo -n lxd init --auto
    abortIfNonZero $? "command 'lxd init --auto'"


    configureLxdNetworking

    if [ "${lxcFs}" = 'zfs' ] ; then
        configureLxdZfs
    fi

    if [ -n "${isServer}" ] ; then
        sudo -n lxc config set core.https_address [::]:8443
        abortIfNonZero $? "command 'lxc config set core.https_address [::]:8443'"
    fi

    sbServerRemote=$(sudo -n lxc remote list | awk '{print $2}' | grep -v '^$' | sed 1d | grep '^sb-server$' | wc -l)
    if [ ${sbServerRemote} -ne 1 ] ; then
        sudo -n lxc remote add --accept-certificate --public sb-server ${SB_SSH_HOST}
        abortIfNonZero $? "command 'lxc remote add --accept-certificate --public sb-server ${SB_SSH_HOST}' on $(hostname --fqdn)"
    fi

    # TODO: Find out what cmd creates .config and run it to ensure the
    # directory will exist.
    if [ -d "${HOME}/.config" ] ; then
        sudo -n cp -a "${HOME}/.config" /root/
        abortIfNonZero $? "command 'cp -a ${HOME}/.config /root/'"
    else
        echo 'info: no ${HOME}/.config directory found'
    fi
}

function prepareNode() {
    # @param $1 $device to format and use for new mount.
    # @param $2 $lxcFs lxc filesystem to use (zfs, btrfs are both supported).
    # @param $3 $zfsPool ZFS pool name.
    # @param $4 $swapDevice to format and use as for swap (optional).
    # @param $5 $isServer (optional).
    local device=${1:-}
    local lxcFs=${2:-}
    local zfsPool=${3:-}
    local swapDevice=${4:-}
    local isServer=${5:-}

    test -z "${device}" && echo 'error: prepareNode() missing required parameter: $device' 1>&2 && exit 1 || :
    test "${device}" = "${swapDevice}" && echo 'error: prepareNode() device & swapDevice must be different' 1>&2 && exit 1 || :
    test ! -e "${device}" && echo "error: unrecognized device '${device}'" 1>&2 && exit 1 || :
    test -z "${lxcFs}" && echo 'error: prepareNode() missing required parameter: $lxcFs' 1>&2 && exit 1 || :
    test "${lxcFs}" = 'zfs' && test -z "${zfsPool}" && echo 'error: prepareNode() missing required zfs parameter: $zfsPool' 1>&2 && exit 1 || :

    local fs
    local zfsPoolArg
    local majorVersion
    local numLines
    local insertAtLineNo

    setupSysctlAndLimits

    if [ -n "${swapDevice}" ] ; then
        echo "info: attempting to unmount swap device=${swapDevice} to be cautious/safe"
        sudo -n umount "${swapDevice}" 1>&2 2>/dev/null
    fi

    # Strip leading '/' from $zfsPool, this is actually a compatibility update for 2017.
    zfsPoolArg="$(echo "${zfsPool}" | sed 's/^\///')"

    installLxc "${lxcFs}" "${zfsPoolArg}" "${device}"

    # Enable zfs snapshot listing.
    sudo -n zpool set listsnapshots=on "${zfsPoolArg}"
    abortIfNonZero $? "command 'zpool set listsnapshots=on ${zfsPoolArg}'"

    prepareZfsDirs

    # Chmod 777 /${zfsPoolArg}/git
    sudo -n chmod 777 '/git'
    abortIfNonZero $? "command 'chmod 777 /git'"

    if ! [ -z "${swapDevice}" ] && [ -e "${swapDevice}" ] ; then
        echo "info: activating swap device or partition: ${swapDevice}"
        # Ensure the swap device target us unmounted.
        sudo -n umount "${swapDevice}" 1>/dev/null 2>/dev/null || :
        # Purge any pre-existing fstab entries before adding the swap device.
        sudo -n sed -i "/^$(echo "${swapDevice}" | sed 's/\//\\&/g')[ \t].*/d" /etc/fstab
        abortIfNonZero $? "purging pre-existing ${swapDevice} entries from /etc/fstab"
        echo "${swapDevice} none swap sw 0 0" | sudo -n tee -a /etc/fstab 1>/dev/null
        sudo -n swapoff --all
        abortIfNonZero $? "adding ${swapDevice} to /etc/fstab"
        sudo -n mkswap -f "${swapDevice}"
        abortIfNonZero $? "command 'mkswap ${swapDevice}'"
        sudo -n swapon --all
        abortIfNonZero $? "command 'swapon --all'"
    fi

    # Install updated kernel if running Ubuntu 12.x series so 'lxc exec' will work.
    majorVersion=$(lsb_release --release | sed 's/^[^0-9]*\([0-9]*\)\..*$/\1/')
    if [ ${majorVersion} -eq 12 ] ; then
        echo 'info: installing 3.8 or newer kernel, a system restart will be required to complete installation'
        sudo -n apt install --yes linux-generic-lts-raring-eol-upgrade
        abortIfNonZero $? 'installing linux-generic-lts-raring-eol-upgrade'
        echo 1 | sudo -n tee -a /tmp/SB_RESTART_REQUIRED
    fi

    echo 'info: installing automatic zpool importer to system startup script /etc/rc.local'
    if [ ! -e /etc/rc.local ] ; then
        sudo -n rm -rf /etc/rc.local
        echo '#!/bin/sh -e' | sudo -n sudo tee /etc/rc.local
        abortIfNonZero $? 'creating /etc/rc.local'
    fi
    sudo -n chmod a+x /etc/rc.local

    if [ $(grep '^sudo[ a-zA-Z0-9-]* zpool import -f '"${zfsPoolArg}"'$' /etc/rc.local | wc -l) -eq 0 ] ; then
        # Find best line number offset to insert the zpool import command at.
        numLines=$(wc --lines /etc/rc.local | awk '{print $1}')
        # Ensure there are enough lines to insert at the minimum line offset.
        if [ ${numLines} -lt 2 ] ; then
            echo '' | sudo tee -a /etc/rc.local
        fi
        # Try to find the first non-comment line number in the file and attempt
        # to insert there.
        insertAtLineNo=$(grep --line-number --max-count=1 '^\([^#]\|$\)' /etc/rc.local | tr ':' ' ' | awk '{print $1}')
        if [ -z "${insertAtLineNo}" ] || [ "${numLines}" = "${insertAtLineNo}" ] ; then
            insertAtLineNo=2
        fi
        sudo -n sed -i "${insertAtLineNo}isudo --non-interactive zpool import -f ${zfsPoolArg}" /etc/rc.local
        abortIfNonZero $? 'adding zpool auto-import to /etc/rc.local'
    fi
    grep "^sudo[ a-zA-Z0-9-]* zpool import -f ${zfsPoolArg}"'$' /etc/rc.local
    abortIfNonZero $? 'ensuring zpool auto-import exists in /etc/rc.local'

    echo 'info: prepareNode() succeeded'
}

function prepareLoadBalancer() {
    local certFile
    local version
    local ppa
    local required
    local optional

    # @param $1 ssl certificate base filename (without path).
    if [ -n "${1:-}" ] ; then
        certFile="/tmp/${1}"
    else
        certFile=
    fi

    version=$(lsb_release -a 2>/dev/null | grep "Release" | grep -o "[0-9\.]\+$")

    if [ "${version}" = "16.04" ] ; then
        ppa='ppa:vbernat/haproxy-1.8'
    elif [ "${version}" = "14.04" ] || [ "${version}" = "13.10" ] || [ "${version}" = "12.04" ] ; then
        ppa='ppa:vbernat/haproxy-1.5'
    elif [ "${version}" = "13.04" ] ; then
        ppa='ppa:nilya/haproxy-1.5'
    else
        echo "error: unrecognized version of ubuntu: ${version}" 1>&2 && exit 1
    fi

    echo '# HAProxy note: The number of queuable sockets is determined by the min of (net.core.somaxconn, net.ipv4.tcp_max_syn_backlog, and the listen blocks maxconn).
# Source: http://stackoverflow.com/questions/8750518/difference-between-global-maxconn-and-server-maxconn-haproxy
net.core.somaxconn = 32000
net.ipv4.tcp_max_syn_backlog = 32000
net.core.netdev_max_backlog = 32000
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216' | sudo -n tee /etc/sysctl.d/60-shipbuilder.conf
    abortIfNonZero $? 'writing out /etc/sysctl.d/60-shipbuilder.conf'

    sudo -n systemctl restart procps
    abortIfNonZero $? 'service procps restart'

    echo "info: adding ppa repository for ${version}: ${ppa}"
    sudo -n apt-add-repository --yes "${ppa}"
    abortIfNonZero $? "command 'apt-add-repository --yes ${ppa}'"

    required="haproxy ntp"
    echo "info: installing required packages: ${required}"
    sudo -n apt update
    abortIfNonZero $? "updating apt"
    sudo -n apt install --yes ${required}
    abortIfNonZero $? "command 'apt install --yes ${required}'"

    optional="vim-haproxy"
    echo "info: installing optional packages: ${optional}"
    sudo -n apt install --yes ${optional}
    abortIfNonZero $? "command 'apt install --yes ${optional}'"

    if [ -n "${certFile}" ] && [ -r "${certFile}" ] ; then
        if ! [ -d "/etc/haproxy/certs.d" ] ; then
            sudo -n mkdir /etc/haproxy/certs.d 2>/dev/null
            abortIfNonZero $? "creating /etc/haproxy/certs.d directory"
        fi
        sudo -n chmod 750 /etc/haproxy/certs.d
        abortIfNonZero $? "chmod 750 /etc/haproxy/certs.d"

        echo "info: installing ssl certificate to /etc/haproxy/certs.d"
        sudo -n mv ${certFile} /etc/haproxy/certs.d/
        abortIfNonZero $? "moving certificate to /etc/haproxy/certs.d"

        sudo -n chmod 400 /etc/haproxy/certs.d/$(echo ${certFile} | sed "s/^.*\/\(.*\)$/\1/")
        abortIfNonZero $? "chmod 400 /etc/haproxy/certs.d/<cert-file>"

        sudo -n chown -R haproxy:haproxy /etc/haproxy/certs.d
        abortIfNonZero $? "chown haproxy:haproxy /etc/haproxy/certs.d"

    else
        echo "warn: no certificate file was provided, ssl support will not be available" 1>&2
    fi

    echo "info: enabling the HAProxy system service in /etc/default/haproxy"
    sudo -n sed -i "s/ENABLED=0/ENABLED=1/" /etc/default/haproxy
    abortIfNonZero $? "enabling haproxy service in /dev/default/haproxy"
    echo 'info: prepareLoadBalancer() succeeded'
}

function installGo() {
    local downloadUrl

    if [ -z "$(command -v go)" ] ; then
        echo "info: installing go v${goVersion}"
        downloadUrl="https://storage.googleapis.com/golang/go${goVersion}.linux-amd64.tar.gz"
        echo "info: downloading go binary distribution from url=${downloadUrl}"
        curl --silent --show-error -o "go${goVersion}.tar.gz" "${downloadUrl}"
        abortIfNonZero $? "downloading go-lang binary distribution"
        sudo -n tar -C /usr/local -xzf "go${goVersion}.tar.gz"
        abortIfNonZero $? "decompressing and installing go binary distribution to /usr/local"
        sudo -n tee /etc/profile.d/Z99-go.sh << EOF
export GOROOT='/usr/local/go'
export GOPATH="\${HOME}/go"
export PATH="\${PATH}:\${GOROOT}/bin:\${GOPATH}/bin"
EOF
        abortIfNonZero $? "creating /etc/profile.d/Z99-golang.sh"
        source /etc/profile.d/Z99-go.sh
        mkdir -p "${GOPATH}/bin" 2>/dev/null
        abortIfNonZero $? "creating directory ${GOPATH}/bin"
    else
        echo 'info: go already appears to be installed, not going to force it'
    fi
}

function rsyslogLoggingListeners() {
    echo 'info: enabling rsyslog listeners'
    echo '# UDP syslog reception.
    $ModLoad imudp
    $UDPServerAddress 0.0.0.0
    $UDPServerRun 514

    # TCP syslog reception.
    $ModLoad imtcp
    $InputTCPServerRun 10514' | sudo -n tee /etc/rsyslog.d/49-haproxy.conf
    echo 'info: restarting rsyslog'
    sudo -n systemctl restart rsyslog
    if [ -e /etc/rsyslog.d/haproxy.conf ] ; then
        echo 'info: detected existing rsyslog haproxy configuration, will disable it'
        sudo -n mv /etc/rsyslog.d/haproxy.conf /etc/rsyslog.d-haproxy.conf.disabled
    fi
    echo 'info: rsyslog configuration succeeded'
}

function getContainerIp() {
    local container=${1:-}

    test -z "${container}" && echo 'error: getContainerIp() missing required parameter: $container' 1>&2 && exit 1 || :

    local allowedAttempts
    local i
    local maybeIp
    local ip

    allowedAttempts=60
    i=0
    ip=''

    echo "info: getting container ip-address for name '${container}'"
    while [ ${i} -lt ${allowedAttempts} ] ; do
        maybeIp="$(sudo -n lxc list --format=json | jq -r ".[] | select(.name==\"${container}\") | .state.network.eth0.addresses[].address" | grep '[0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}')"
        # Verify that after a few seconds the ip hasn't changed.
        if [ -n "${maybeIp}" ] ; then
            sleep 1
            ip="$(sudo -n lxc list --format=json | jq -r ".[] | select(.name==\"${container}\") | .state.network.eth0.addresses[].address" | grep '[0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}')"
            if [ "${ip}" = "${maybeIp}" ] ; then
                echo "info: ip-address verified, value=${ip}"
                break
            else
                echo "warn: ip-address not stable try1=${maybeIp} try2=${ip}"
            fi
            echo "info: found an ip=${ip}"
        fi
        i=$(($i+1))
        sleep 1
    done
    if [ -n "${ip}" ] ; then
        echo "info: found ip=${ip} for ${container} container"
    else
        echo "error: obtaining ip-address for container=${container} failed after ${allowedAttempts} attempts" 1>&2
        exit 1
    fi
}

function lxcInitContainer() {
    # @param $1 container name.
    # @param $2 skipIfExists Whether or not to skip over this container if it already exists.
    # @param $3 lxc filesystem to use.
    # @param $4 zfs pool name.
    local container=${1:-}
    local skipIfExists=${2:-}
    local lxcFs=${3:-}

    test -z "${container}" && echo 'error: lxcInitContainer() missing required parameter: $container' 1>&2 && exit 1
    test -z "${skipIfExists}" && echo 'error: lxcInitContainer() missing required parameter: $skipIfExists' 1>&2 && exit 1
    test -z "${lxcFs}" && echo 'error: lxcInitContainer() missing required parameter: $lxcFs' 1>&2 && exit 1

    local existsRc

    lxcContainerExists "${container}"
    existsRc=$?

    if [ ${skipIfExists} -eq 1 ] && [ ${existsRc} -eq 0 ] ; then
        echo "info: lxcInitContainer() skipping container=${container} because it already exists and the skip flag was passed"
    else
        echo "info: clearing any pre-existing container=${container}"
        sudo -n lxc delete --force "${container}"

        echo "info: creating lxc container=${container}"
        echo -e '\n' | sudo -n lxc launch "${lxcBaseImage}" "${container}"

        getContainerIp "${container}"

        lxcConfigContainer "${container}"
        abortIfNonZero $? "lxcInitContainer() lxcConfigContainer() failed for container=${container}"
    fi
}

function lxcConfigContainer() {
    # @param $1 container name.
    local container=${1:-}

    test -z "${container}" && echo 'error: lxcConfigContainer() missing required parameter: $container' 1>&2 && exit 1

    local packages

    echo "info: adding shipbuilder server's public-key to authorized_keys file in container=${container}"

    sudo -n lxc exec -T "${container}" -- bash -c "set -o errexit && set -o pipefail && sudo -n -u ubuntu mkdir -p /home/ubuntu/.ssh && chown -R ubuntu:ubuntu /home/ubuntu/.ssh && chmod 700 /home/ubuntu/.ssh"
    abortIfNonZero $? "creation of container=${container} ~/.ssh directory"

    sudo -n lxc exec -T "${container}" -- sudo -n -u ubuntu ls -lah /home/ubuntu/

    sudo -n lxc exec -T "${container}" -- sudo -n -u ubuntu tee /home/ubuntu/.ssh/authorized_keys < ~/.ssh/id_rsa.pub
    abortIfNonZero $? "creation of container=${container} ssh authorized_keys"

    sudo -n lxc exec -T "${container}" -- chmod 600 /home/ubuntu/.ssh/authorized_keys
    abortIfNonZero $? "chmod 600 container=${container} .ssh/authorized_keys"

    echo 'info: adding the container "ubuntu" user to the sudoers list'
    sudo -n lxc exec -T "${container}" -- sudo -n bash -c 'set -o errexit && set -o pipefail && echo "ubuntu ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers'
    abortIfNonZero $? "adding 'ubuntu' to container=${container} sudoers"

    echo "info: updating apt repositories in container=${container}"
    # ssh -o 'StrictHostKeyChecking=no' -o 'BatchMode=yes' "ubuntu@${ip}" "sudo -n apt update"
    sudo -n lxc exec -T "${container}" -- /bin/bash -c "${SB_DEBUG_BASH} && set -o errexit && DEBIAN_FRONTEND=noninteractive apt update"
    abortIfNonZero $? "container=${container} apt update"

    packages='daemontools git-core curl unzip'
    echo "info: installing packages to container=${container}: ${packages}"
    sudo -n lxc exec -T "${container}" -- /bin/bash -c "${SB_DEBUG_BASH} && set -o errexit && DEBIAN_FRONTEND=noninteractive apt install --yes ${packages}"
    abortIfNonZero $? "container=${container} apt install --yes ${packages}"

    echo "info: removing $(shipbuilder containers list-purge-packages | tr $'\n' ' ') packages"
    sudo -n lxc exec -T "${container}" -- /bin/bash -c "${SB_DEBUG_BASH} && set -o errexit && DEBIAN_FRONTEND=noninteractive apt purge --yes $(shipbuilder containers list-purge-packages | tr $'\n' ' ')"
    abortIfNonZero $? "container=${container} apt purge --yes $(shipbuilder containers list-purge-packages | tr $'\n' ' ')"

    echo "info: disabling unnecessary system services - $(shipbuilder containers list-disable-services | tr $'\n' ' ')"
    shipbuilder containers list-disable-services | sudo -n lxc exec -T "${container}" -- \
        /bin/bash -c "${SB_DEBUG_BASH} && set -o errexit && xargs -n 1 -IX /bin/bash -c 'systemctl is-enabled X 1>/dev/null ; test \$? -ne 0 && return 0 ; systemctl stop X ; set -o errexit ; systemctl disable X'"
    abortIfNonZero $? "container=${container} disabling unnecessary system services - $(shipbuilder containers list-disable-services | tr $'\n' ' ')"

    echo "info: stopping container=${container}"
    sudo -n lxc stop --force "${container}"

    echo "info: configuration succeeded for container=${container}"
}

function lxcContainerExists() {
    local container=${1:-}

    if [ -z "${container}" ]; then
        echo 'error: lxcContainerExists() missing required parameter: $container' 1>&2
        exit 1
    fi

    # Test whether or not the container already exists.
    if [ -z "$(sudo -n lxc list --format=json | jq -r ".[] | select(.name==\"${container}\")")" ]; then
        return 1
    fi
}

function lxcContainerRunning() {
    local container=${1:-}

    test -z "${container}" && echo 'error: lxcContainerRunning() missing required parameter: $container' 1>&2 && exit 1

    # Test whether or not the container already exists and is running.
    test "$(sudo -n lxc list --format=json | jq -r ".[] | select(.name==\"${container}\") | .status")" = 'Running'
}

function lxcDestroyContainer() {
    local  __resultvar=$1
    local container=${2:-}
    local lxcFs=${3:-}

    test -z "${container}" && echo 'error: lxcDestroyContainer() missing required parameter: $container' 1>&2 && exit 1
    test -z "${lxcFs}" && echo 'error: lxcDestroyContainer() missing required parameter: $lxcFs' 1>&2 && exit 1

    local result
    local existsRc
    local attempts

    result=0

    lxcContainerExists "${container}"
    existsRc=$?
    if [ ${existsRc} -eq 0 ] ; then
        sudo -n lxc stop --force "${container}"
        local attempts=10
        while [ ${attempts} -gt 0 ] ; do
            sudo -n lxc delete --force "${container}"
            test $? -eq 0 && break
            attempts=$((${attempts}-1))
        done
        if [ ${attempts} -eq 0 ] ; then
            abortWithError "Max attempts to stop container=${container} exceeded"
        fi
        lxcContainerExists "${container}"
        existsRc=$?
        if [ ${existsRc} -eq 0 ] ; then
            echo "info: lxcDestroyContainer() failed to destroy container=${container}"
            result=1
        else
            # Ensure zfs volume gets destroyed.
            test "${lxcFs}" = 'zfs' && sudo -n zfs destroy "tank/base@${container}" || true
            echo "info: lxcDestroyContainer() successfully destroyed container=${container}"
        fi
    else
        echo "info: lxcDestroyContainer() failed because container=${container} does not exist"
    fi
    eval $__resultvar="'${result}'"
}

function lxcConfigBuildPack() {
    # @param $1 build-pack name which will become the base-container suffix (e.g. 'python').
    # @param $2 skipIfExists Whether or not to skip over this container if it already exists.
    # @param $3 lxc filesystem to use.
    # @param $4 zfs pool (optional, only needed when applicable).
    local buildPack=${1:-}
    local skipIfExists=${2:-}
    local lxcFs=${3:-}

    test -z "${buildPack}" && echo 'error: lxcConfigBuildPack() missing required parameter: $buildPack' 1>&2 && exit 1 || :
    test -z "${skipIfExists}" && echo 'error: lxcConfigBuildPack() missing required parameter: $skipIfExists' 1>&2 && exit 1 || :
    test -z "${lxcFs}" && echo 'error: lxcConfigBuildPack() missing required parameter: $lxcFs' 1>&2 && exit 1 || :

    local container
    local packagesFile
    local packages
    local customCommandsFile
    local runningRc
    local existsRc
    local rc

    local container="base-${buildPack}"

    packagesFile="${SB_REPO_PATH}/build-packs/${buildPack}/container-packages"
    test ! -r "${packagesFile}" && echo "error: lxcConfigBuildPack() missing packages file for build-pack '${buildPack}': '${packagesFile}' not found" 1>&2 && exit 1
    packages="$(cat "${packagesFile}" 2>/dev/null | tr -d '\n')"

    customCommandsFile="${SB_REPO_PATH}/build-packs/${buildPack}/container-custom-commands"
    test ! -r "${customCommandsFile}" && echo "error: lxcConfigBuildPack() missing custom commands file for build-pack '${buildPack}': '${customCommandsFile}' not found" 1>&2 && exit 1

    # Test if the container was left in a running state, and if so, destroy it (since failed runs can leave things partially done).
    lxcContainerRunning "${container}"
    runningRc=$?
    if [ ${runningRc} -eq 0 ] ; then
        lxcDestroyContainer destroyedRc "${container}" "${lxcFs}"
        test ${destroyedRc} -ne 0 && echo "error: failed to destroy container=${container}" 1>&2 && exit 1
    fi

    lxcContainerExists "${container}"
    existsRc=$?
    if [ "${skipIfExists}" = '1' ] && [ ${existsRc} -eq 0 ] ; then
        echo "info: lxcConfigBuildPack() skipping container=${container} because it already exists and skipIfExists=${skipIfExists}"
    else
        test -z "${lxcFs}" && echo 'error: lxcConfigBuildPack() missing required parameter: $lxcFs' 1>&2 && exit 1
        echo "info: creating build-pack ${container} container"

        # Ensure any pre-existing image gets removed.
        sudo -n lxc delete --force "${container}" 1>/dev/null 2>/dev/null

        sudo -n lxc copy base "${container}"
        abortIfNonZero $? "command 'lxc copy base ${container}'"

        sudo -n lxc start "${container}"
        abortIfNonZero $? "command 'lxc start ${container}"

        getContainerIp "${container}"

        # Install packages.
        echo "info: installing packages to ${container} container: ${packages}"
        sudo -n lxc exec -T "${container}" -- /bin/bash -c "set -o errexit && apt update && apt -o Dpkg::Options::='--force-overwrite' install --yes ${packages}"
        rc=$?
        if [ ${rc} -ne 0 ] ; then
            echo 'warning: first attempt at installing packages failed, falling back to trying installation of packages one by one..'
            for package in ${packages} ; do
                #ssh -o 'StrictHostKeyChecking=no' -o 'BatchMode=yes' "ubuntu@${ip}" "sudo -n apt install --yes ${package}"
                sudo -n lxc exec -T "${container}" -- /bin/bash -c "set -o errexit && apt install -o Dpkg::Options::='--force-overwrite' --yes ${package}"
                abortIfNonZero $? "[${container}] container apt install --yes ${package}"
            done
        fi

        # Run custom container commands.
        if [ -n "${customCommandsFile}" ] ; then
            if [ ! -r "${customCommandsFile}" ] ; then
                echo "error: lxcConfigBuildPack(buildPack=${buildPack}, skipIfExists=${skipIfExists}, lxcFs=${lxcFs}): unable to read customCommandsFile=${customCommandsFile}" 1>&2 && exit 1
            fi
            echo "info: running customCommandsFile: ${customCommandsFile}"
            base64 < "${customCommandsFile}" | sudo -n lxc exec -T "${container}" -- /bin/bash -c "set -o errexit && base64 -d > /tmp/custom.sh && chmod a+x /tmp/custom.sh"
            abortIfNonZero $? "[${container}] sending customCommandsFile=${customCommandsFile} to ${container} failed"
            sudo -n lxc exec -T "${container}" -- /bin/bash /tmp/custom.sh
            rc=$?
            # Cleanup temp custom commands script.
            sudo -n lxc exec -T "${container}" -- rm -f /tmp/custom.sh
            abortIfNonZero ${rc} "[${container}] container customCommandsFile=${customCommandsFile}"
        fi

        echo "info: stopping ${container} container"
        sudo -n lxc stop --force "${container}"
        abortIfNonZero $? "[${container}] lxc stop --force ${container}"

        echo 'info: build-pack configuration succeeded'
    fi
}

function lxcConfigBuildPacks() {
    # @param $2 lxc filesystem to use.
    local skipIfExists=${1:-}
    local lxcFs=${2:-}

    test -z "${lxcFs}" && echo 'error: lxcConfigBuildPacks() missing required parameter: $lxcFs' 1>&2 && exit 1 || :

    for buildPack in $(ls -1 "${SB_REPO_PATH}/build-packs") ; do
        echo "info: initializing build-pack: ${buildPack}"
        # NB: "tr -d" is very important here to prevent invalid package name being installed (last package will have a trailing newline).
        lxcConfigBuildPack "${buildPack}" "${skipIfExists}" "${lxcFs}"
        echo 'info: build-pack initialized succeeded'
    done
}

function prepareServerPart1() {
    # @param $1 ShipBuilder server hostname or ip-address.
    # @param $2 device to format and use for new mount.
    # @param $3 lxc filesystem to use.
    # @param $4 $zfsPool zfs pool name to create (only required when lxc filesystem is zfs).
    # @param $5 $swapDevice to format and use as for swap (optional).
    local sbHost=${1:-}
    local device=${2:-}
    local lxcFs=${3:-}
    local zfsPool=${4:-}
    local swapDevice=${5:-}

    test -z "${sbHost}" && echo 'error: prepareServerPart1(): missing required parameter: shipbuilder host' 1>&2 && exit 1 || :
    test -z "${device}" && echo 'error: prepareServerPart1(): missing required parameter: device' 1>&2 && exit 1 || :
    test -z "${lxcFs}" && echo 'error: prepareServerPart1(): missing required parameter: lxcFs' 1>&2 && exit 1 || :
    test "${device}" = "${swapDevice}" && echo 'error: prepareServerPart1() device & swapDevice must be different' 1>&2 && exit 1 || :
    test "${lxcFs}" = 'zfs' && test -z "${zfsPool}" && echo 'error: prepareServerPart1() missing required zfs parameter: $zfsPool' 1>&2 && exit 1 || :

    prepareNode "${device}" "${lxcFs}" "${zfsPool}" "${swapDevice}" "1"
    abortIfNonZero $? 'prepareNode() failed'

    installGo
    abortIfNonZero $? 'installGo() failed'

    rsyslogLoggingListeners
    abortIfNonZero $? 'rsyslogLoggingListeners() failed'

    echo 'info: prepareServerPart1() succeeded'
}

function prepareServerPart2() {
    # @param $2 lxc filesystem to use.
    local skipIfExists=${1:-}
    local lxcFs=${2:-}

    test -z "${skipIfExists}" && echo 'error: prepareServerPart2() missing required parameter: $skipIfExists' 1>&2 && exit 1 || :
    test -z "${lxcFs}" && echo 'error: prepareServerPart2() missing required parameter: $lxcFs' 1>&2 && exit 1 || :

    local container='base'

    lxcInitContainer "${container}" "${skipIfExists}" "${lxcFs}"
    abortIfNonZero $? "lxcInitContainer(container=${container}, lxcFs=${lxcFs}, skipIfExists=${skipIfExists}) failed"

    lxcConfigBuildPacks "${skipIfExists}" "${lxcFs}"
    abortIfNonZero $? "lxcConfigBuildPacks(lxcFs=${lxcFs}, skipIfExists=${skipIfExists}) failed"

    echo 'info: prepareServerPart2() succeeded'
}

function installSingleBuildPack() {
    # @param $3 lxc filesystem to use.
    local buildPack=${1:-}
    local skipIfExists=${2:-}
    local lxcFs=${3:-}

    test -z "${buildPack}" && echo 'error: installSingleBuildPack() missing required parameter: $buildPack' 1>&2 && exit 1
    test -z "${skipIfExists}" && echo 'error: installSingleBuildPack() missing required parameter: $skipIfExists' 1>&2 && exit 1
    test -z "${lxcFs}" && echo 'error: installSingleBuildPack() missing required parameter: $lxcFs' 1>&2 && exit 1

    lxcConfigBuildPack "${buildPack}" "${skipIfExists}" "${lxcFs}"

    echo 'info: installSingleBuidlPack() succeeded'
}
