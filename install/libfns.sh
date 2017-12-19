set -x

export SB_REPO_PATH="${GOPATH:-${HOME}/go}/src/github.com/jaytaylor/shipbuilder"

export goVersion='1.9.1'
export lxcBaseImage='ubuntu:16.04'

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

function abortIfNonZero() {
    # @param $1 command return code/exit status (e.g. $?, '0', '1').
    # @param $2 error message if exit status was non-zero.
    local rc=$1
    local what=$2
    test $rc -ne 0 && echo "error: ${what} exited with non-zero status ${rc}" 1>&2 && exit $rc || :
}

function abortWithError() {
    echo "$1" 1>&2 && exit 1
}

function warnIfNonZero() {
    # @param $1 command return code/exit status (e.g. $?, '0', '1').
    # @param $2 error message if exit status was non-zero.
    local rc=$1
    local what=$2
    test $rc -ne 0 && echo "warn: ${what} exited with non-zero status ${rc}" 1>&2 || :
}

function autoDetectServer() {
    # Attempts to auto-detect the server host by reading the contents of ../env/SB_SSH_HOST.
    if [ -r '../env/SB_SSH_HOST' ] ; then
        sbHost=$(head -n1 ../env/SB_SSH_HOST)
        test -n "${sbHost}" && echo "info: auto-detected shipbuilder host: ${sbHost}"
    else
        echo 'warn: server auto-detection failed: no such file: ../env/SB_SSH_HOST' 1>&2
    fi
}

function autoDetectFilesystem() {
    # Attempts to auto-detect the target filesystem type by reading the contents of ../env/LXC_FS.
    if [ -r '../env/SB_LXC_FS' ] ; then
        lxcFs=$(head -n1 ../env/SB_LXC_FS)
        test -n "${lxcFs}" && echo "info: auto-detected lxc filesystem: ${lxcFs}"
    else
        echo 'warn: lxc filesystem auto-detection failed: no such file: ../env/SB_LXC_FS' 1>&2
    fi
}

function autoDetectZfsPool() {
    # When fs type is 'zfs', attempt to auto-detect the zfs pool name to create by reading the contents of ../env/ZFS_POOL.
    test -z "${lxcFs}" && autoDetectFilesystem # Attempt to ensure that the target filesystem type is available.
    if [ "${lxcFs}" = 'zfs' ] ; then
        if [ -r '../env/SB_ZFS_POOL' ] ; then
            zfsPool="$(head -n1 ../env/SB_ZFS_POOL)"
            test -n "${zfsPool}" && echo "info: auto-detected zfs pool: ${zfsPool}"
            # Validate to ensure zfs pool name won't conflict with typical ubuntu root-fs items.
            for x in bin boot dev etc git home lib lib64 media mnt opt proc root run sbin selinux srv sys tmp usr var vmlinuz zfs-kstat ; do
                test "${zfsPool}" = "${x}" && echo "error: invalid zfs pool name detected, '${x}' is a forbidden because it may conflict with a system directory" 1>&2 && exit 1
            done
        else
            echo 'warn: zfs pool auto-detection failed: no such file: ../env/SB_ZFS_POOL' 1>&2
        fi
    fi
}

function verifySshAndSudoForHosts() {
    # @param $1 string. List of space-delimited SSH connection strings.
    local sshHosts="$1"
    echo "info: verifying ssh and sudo access for $(echo "${sshHosts}" | tr ' ' '\n' | grep -v '^ *$' | wc -l | sed 's/^[ \t]*//g') hosts"
    for sshHost in $(echo "${sshHosts}") ; do
        echo -n "info:     testing host ${sshHost} .. "
        result=$(ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' -o 'ConnectTimeout=15' -q "${sshHost}" 'sudo -n echo "succeeded" 2>/dev/null')
        rc=$?
        test $rc -ne 0 && echo 'failed' && abortWithError "error: ssh connection test failed for host: ${sshHost} (exited with status code: ${rc})"
        test -z "${result}" && echo 'failed' && abortWithError "error: sudo access test failed for host: ${sshHost}"
        echo 'succeeded'
    done
}

function initSbServerKeys() {
    # @precondition $sbHost must not be empty.
    test -z "${sbHost}" && echo 'error: initSbServerKeys(): required parameter $sbHost cannot be empty' 1>&2 && exit 1
    echo "info: checking SB server=${sbHost} SSH keys, will generate if missing"

    ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' $sbHost '/bin/bash -c '"'"'
    echo "remote: info: setting up pub/private SSH keys so that root and main users can SSH in to either account"
    function abortIfNonZero() {
        local rc=$1
        local what=$2
        test $rc -ne 0 && echo "remote: error: ${what} exited with non-zero status ${rc}" && exit $rc || :
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

    if ! sudo test -e /root/.ssh ; then
        sudo mkdir /root/.ssh
        sudo chmod 700 ~/.ssh
    fi
    if ! sudo test -e /root/.ssh/authorized_keys || sudo test -z "$(sudo grep "$(cat ~/.ssh/id_rsa.pub)" /root/.ssh/authorized_keys)" ; then
        echo "remote: info: adding main user to root user authorized_keys"
        cat ~/.ssh/id_rsa.pub | sudo tee -a /root/.ssh/authorized_keys >/dev/null
        abortIfNonZero $? "appending public-key to authorized_keys command"
        sudo chmod 600 /root/.ssh/authorized_keys
        abortIfNonZero $? "chmod 600 /root/.ssh/authorized_keys command"
    fi

    if ! sudo test -e /root/.ssh/id_rsa.pub ; then
        echo "remote: info: generating a new private/public key-pair for root user"
        if [ -n "$(sudo bash -c "/root/.ssh/id_*")" ] ; then
            backupDir="sb_backup_$(date +%s)"
            sudo mkdir "/root/.ssh/${backupDir}"
            sudo bash -c "mv /root/.ssh/id_* /root/.ssh/${backupDir}/"
            abortIfNonZero $? "backing up old keys failed"
        fi
        sudo ssh-keygen -f /root/.ssh/id_rsa -t rsa -N ""
        abortIfNonZero $? "ssh-keygen command failed"
    fi
    if [ test -z "$(sudo grep "$(sudo cat /root/.ssh/id_rsa.pub)" /root/.ssh/authorized_keys)" ] ; then
        echo "remote: info: adding root to root user authorized_keys"
        sudo cat /root/.ssh/id_rsa.pub | sudo tee -a /root/.ssh/authorized_keys >/dev/null
        abortIfNonZero $? "appending public-key to authorized_keys command"
        sudo chmod 600 /root/.ssh/authorized_keys
        abortIfNonZero $? "chmod 600 /root/.ssh/authorized_keys command"
    fi
    if [ -z "$(grep "$(sudo cat /root/.ssh/id_rsa.pub)" ~/.ssh/authorized_keys)" ] ; then
        echo "remote: info: adding root to main user authorized_keys"
        sudo cat /root/.ssh/id_rsa.pub >> ~/.ssh/authorized_keys
        abortIfNonZero $? "appending public-key to authorized_keys command"
        sudo chmod 600 ~/.ssh/authorized_keys
        abortIfNonZero $? "chmod 600 ~/.ssh/authorized_keys command"
    fi'"'"
    abortIfNonZero $? 'ssh key initialization'
    echo 'info: ssh key initialization succeeded'
}

function getSbServerPublicKeys() {
    test -z "${sbHost}" && echo 'error: getSbServerPublicKeys(): missing required parameter: SSH hostname' 1>&2 && exit 1

    initSbServerKeys

    echo "info: retrieving public-keys from shipbuilder server: ${sbHost}"

    local pubKeys=$(ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' $sbHost 'cat ~/.ssh/id_rsa.pub && echo "." && sudo cat /root/.ssh/id_rsa.pub')
    abortIfNonZero $? 'SSH public-key retrieval failed'

    unprivilegedPubKey=$(echo "${pubKeys}" | grep --before 100 '^\.$' | grep -v '^\.$')
    rootPubKey=$(echo "${pubKeys}" | grep --after 100 '^\.$' | grep -v '^\.$')

    if [ -z "${unprivilegedPubKey}" ] ; then
        echo 'error: failed to obtain build-server public-key for unprivileged user' 1>&2
        exit 1
    fi
    echo "info: obtained unprivileged public-key: ${unprivilegedPubKey}"
    if [ -z "${rootPubKey}" ] ; then
        echo 'error: failed to obtain build-server public-key for root user' 1>&2
        exit 1
    fi
    echo "info: obtained root public-key: ${rootPubKey}"
}

function installAccessForSshHost() {
    # @precondition Variable $sshKeysCommand must be initialized and not empty.
    # @param $1 SSH connection string (e.g. user@host)
    local sshHost=$1

    if [ -z "${unprivilegedPubKey}" ] || [ -z "${rootPubKey}" ] ; then
        getSbServerPublicKeys
    fi

    test -z "${sshHost}" && echo 'error: installAccessForSshHost(): missing required parameter: SSH hostname' 1>&2 && exit 1 || :

    echo "info: setting up remote access from build-server to host: ${sshHost}"
    ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' $sshHost '/bin/bash -c '"'"'
    function abortIfNonZero() {
        local rc=$1
        local what=$2
        test $rc -ne 0 && echo "remote: error: ${what} exited with non-zero status ${rc}" && exit $rc || :
    }
    echo "remote: checking main user.."
    if [ -z "$(grep "'"${unprivilegedPubKey}"'" ~/.ssh/authorized_keys)" ] || [ -z "$(sudo grep "'"${rootPubKey}"'" ~/.ssh/authorized_keys)" ] ; then
        echo -e "'"${unprivilegedPubKey}\n${rootPubKey}"'" >> ~/.ssh/authorized_keys
        abortIfNonZero $? "appending public-keys to authorized_keys command"
        chmod 600 ~/.ssh/authorized_keys
        abortIfNonZero $? "chmod 600 ~/.ssh/authorized_keys command"
    fi
    echo "remote: checking root user.."
    if sudo test -z "$(sudo grep "'"${unprivilegedPubKey}"'" /root/.ssh/authorized_keys)" || sudo test -z "$(sudo grep "'"${rootPubKey}"'" /root/.ssh/authorized_keys)" ; then
        echo -e "'"${unprivilegedPubKey}\n${rootPubKey}"'" | sudo tee -a /root/.ssh/authorized_keys >/dev/null
        abortIfNonZero $? "appending public-keys to authorized_keys command"
        sudo chmod 600 /root/.ssh/authorized_keys
        abortIfNonZero $? "chmod 600 /root/.ssh/authorized_keys command"
    fi
    exit 0
    '"'"
    abortIfNonZero $? "ssh access installation failed for host ${sshHost}"
    echo 'info: ssh access installation succeeded'
}

function installLxc() {
    # @param $1 $lxcFs lxc filesystem to use (zfs, btrfs are both supported).
    local lxcFs=$1
    test -z "${lxcFs}" && echo 'error: installLxc() missing required parameter: $lxcFs' 1>&2 && exit 1 || :
    echo 'info: a supported version of lxc must be installed (as of 2013-07-02, ubuntu comes with v0.7.x by default, we require v0.9.0 or greater)'
    echo 'info: adding lxc daily ppa'
    sudo apt-add-repository -y ppa:ubuntu-lxc/stable
    abortIfNonZero $? "command 'sudo apt-add-repository -y ppa:ubuntu-lxc/stable'"

    sudo apt-get update
    abortIfNonZero $? "command 'sudo add-get update'"

    sudo apt-get install -y lxc lxc-templates
    abortIfNonZero $? "command 'apt-get install -y lxc lxc-templates'"

    echo "info: installed version $(sudo apt-cache show lxc | grep Version | sed 's/^Version: //') (should be >= 0.9.0)"

    # Add supporting package(s) for selected filesystem type.
    local fsPackages="$(test "${lxcFs}" = 'btrfs' && echo 'btrfs-tools' || :) $(test "${lxcFs}" = 'zfs' && echo 'zfsutils-linux' || :)"

    local required="${fsPackages} git mercurial bzr build-essential bzip2 daemontools ntp ntpdate jq"
    echo "info: installing required build-server packages: ${required}"
    sudo apt-get install -y ${required}
    abortIfNonZero $? "command 'apt-get install -y ${required}'"

    local recommended='aptitude htop iotop unzip screen bzip2 bmon'
    echo "info: installing recommended packages: ${recommended}"
    sudo apt-get install -y ${recommended}
    abortIfNonZero $? "command 'apt-get install -y ${recommended}'"
    echo 'info: installLxc() succeeded'
}

function setupSysctlAndLimits() {
    local fsValue='1048576'
    local limitsValue='100000'

    echo 'info: installing config params to /etc/sysctl.conf'
    for param in max_queued_events max_user_instances max_user_watches ; do
        sudo sed -i "/^fs\.inotify\.${param} *=.*\$/d" /etc/sysctl.conf
        abortIfNonZero $? "cleaning config param=${param} from /etc/sysctl.conf"
        echo "fs.inotify.${param} = ${fsValue}" | sudo tee -a /etc/sysctl.conf
        abortIfNonZero $? "setting config param=${param} in /etc/sysctl.conf"
    done

    echo 'info: installing config params to /etc/security/limits.conf'
    for param in soft hard ; do
        sudo sed -i "/^\* ${param} .*\$/d" /etc/security/limits.conf
        abortIfNonZero $? "cleaning config param=${param} from /etc/security/limits.conf"
        echo "* ${param} nofile ${limitsValue}" | sudo tee -a /etc/security/limits.conf
        abortIfNonZero $? "setting config param=${param} in /etc/security/limits.conf"
    done
}

function setupLxdWithZfs() {
    local zfsPoolArg=$1

    test -z "${zfsPoolArg}" && echo 'error: setupLxdWithZfs() missing required parameter: $zfsPoolArg' 1>&2 && exit 1 || :

    sudo lxd init --auto
    abortIfNonZero $? "command 'sudo lxd init --auto'"

    # Setup LXC/LXD networking.
    ip addr show lxdbr0 1>/dev/null 2>/dev/null
    if [ $? -eq 0 ] ; then
        test -n "$(ip addr show lxdbr0 | grep ' inet ')" || sudo lxc network delete lxdbr0
        abortIfNonZero $? "lxc/lxd removal of non-ipv4 network bridge lxdbr0"
    fi

    test -n "$(sudo lxc network list | grep 'lxdbr0')" || sudo lxc network create lxdbr0 ipv6.address=none ipv4.address=10.0.1.1/24 ipv4.nat=true
    abortIfNonZero $? "lxc/lxd ipv5 network bridge creation of lxdbr0"

    lxc network attach-profile lxdbr0 default eth0

    # Create ZFS pool mount point.
    # test ! -d "/${zfsPoolArg}" && sudo rm -rf "/${zfsPoolArg}" && sudo mkdir "/${zfsPoolArg}" || :
    # abortIfNonZero $? "creating /${zfsPool} mount point"

    # TODO: Move to function and invoke even when FS already formatted w/ ZFS.

    if [ -z "$(lxc storage show "${zfsPoolArg}" 2>/dev/null)" ] ; then
        sudo lxc storage create "${zfsPoolArg}" zfs "source=${device}"
        abortIfNonZero $? "command 'sudo lxc storage create ${zfsPoolArg} zfs source=${device}'"
    fi

    if [ -z "$(sudo lxc profile device show default | grep -A3 '^root:' | grep "pool: ${zfsPoolArg}")" ] ; then
        sudo lxc profile device remove default root
        sudo lxc profile device add default root disk path=/ "pool=${zfsPoolArg}"
        abortIfNonZero $? "LXC root zfs device assertion"
    fi
    # sudo lxc profile device show default || sudo lxc profile device add default root disk path=/ "pool=${zfsPoolArg}"
    # abortIfNonZero $? "LXC root zfs device assertion"

   # # Create ZFS pool and attach to a device.
   # if test -z "$(sudo zfs list -o name,mountpoint | sed '1d' | grep "^${zfsPoolArg}.*\/${zfsPoolArg}"'$')" ; then
   #     # Format the device with any filesystem (mkfs.ext4 is fast).
   #     sudo mkfs.ext4 -q "${device}"
   #     abortIfNonZero $? "command 'sudo mkfs.ext4 -q ${device}'"

   #     sudo zpool destroy "${zfsPoolArg}" 2>/dev/null

   #     #sudo zpool create -o ashift=12 "${zfsPoolArg}" "${device}"
   #     #abortIfNonZero $? "command 'sudo zpool create -o ashift=12 ${zfsPoolArg} ${device}'"
   #     sudo zpool create -f "${zfsPoolArg}" "${device}"
   #     abortIfNonZero $? "command 'sudo zpool create -f ${zfsPoolArg} ${device}'"
   # fi

    # Create lxc and git volumes and set mountpoints.
    for volume in git ; do
        #test -z "$(sudo zfs list -o name | sed '1d' | grep "^${zfsPoolArg}\/${volume}")" && sudo zfs create -o compression=on "${zfsPoolArg}/${volume}" || :
        test -n "$(sudo zfs list -o name | sed '1d' | grep "^${zfsPoolArg}\/${volume}")" || sudo zfs create -o compression=on "${zfsPoolArg}/${volume}"
        abortIfNonZero $? "command 'sudo zfs create -o compression=on ${zfsPoolArg}/${volume}'"

        sudo zfs set "mountpoint=/${zfsPoolArg}/${volume}" "${zfsPoolArg}/${volume}"
        abortIfNonZero $? "setting mountpoint via 'zfs set mountpoint=/${zfsPoolArg}/${volume} ${zfsPoolArg}/${volume}'"

        sudo zfs mount "${zfsPoolArg}/${volume}"
        abortIfNonZero $? "zfs mount'ing ${zfsPoolArg}/${volume}"

        sudo unlink "/${volume}" || :

        sudo ln -s "/${zfsPoolArg}/${volume}" "/${volume}"
        abortIfNonZero $? "setting up symlink for volume=${volume}"
    done

    # Mount remaining volumes under LXC base path (rather than $zfsPoolArg
    # [e.g. "/tank"]).
    lxcBasePath=/var/lib/lxd

    for volume in containers images snapshots ; do
        test -n "$(sudo zfs list -o name | sed '1d' | grep "^${zfsPoolArg}\/${volume}")" || sudo zfs create -o compression=on "${zfsPoolArg}/${volume}"
        abortIfNonZero $? "command 'sudo zfs create -o compression=on ${zfsPoolArg}/${volume}'"

        sudo zfs set "mountpoint=${lxcBasePath}/${volume}" "${zfsPoolArg}/${volume}"
        abortIfNonZero $? "setting mountpoint via 'zfs set mountpoint=${lxcBasePath}/${volume} ${zfsPoolArg}/${volume}'"

        sudo zfs mount "${zfsPoolArg}/${volume}"
        abortIfNonZero $? "zfs mount'ing ${zfsPoolArg}/${volume}"

        sudo unlink "/${volume}" || :

        sudo ln -s "/${zfsPoolArg}/${volume}" "/${volume}"
        abortIfNonZero $? "setting up symlink for volume=${volume}"
    done
}

function prepareNode() {
    # @param $1 $device to format and use for new mount.
    # @param $2 $lxcFs lxc filesystem to use (zfs, btrfs are both supported).
    # @param $3 $swapDevice to format and use as for swap (optional).
    local device=$1
    local lxcFs=$2
    local zfsPool=$3
    local swapDevice=$4
    test -z "${device}" && echo 'error: prepareNode() missing required parameter: $device' 1>&2 && exit 1 || :
    test "${device}" = "${swapDevice}" && echo 'error: prepareNode() device & swapDevice must be different' 1>&2 && exit 1 || :
    test ! -e "${device}" && echo "error: unrecognized device '${device}'" 1>&2 && exit 1 || :
    test -z "${lxcFs}" && echo 'error: prepareNode() missing required parameter: $lxcFs' 1>&2 && exit 1 || :
    test "${lxcFs}" = 'zfs' && test -z "${zfsPool}" && echo 'error: prepareNode() missing required zfs parameter: $zfsPool' 1>&2 && exit 1 || :

    setupSysctlAndLimits

    echo "info: attempting to unmount /mnt and ${device} to be cautious/safe"
    sudo umount /mnt 1>&2 2>/dev/null
    sudo umount "${device}" 1>&2 2>/dev/null

    if test -n "${swapDevice}" ; then
        echo "info: attempting to unmount swap device=${swapDevice} to be cautious/safe"
        sudo umount "${swapDevice}" 1>&2 2>/dev/null
    fi

    if ! [ -d '/mnt/build' ] ; then
        echo 'info: creating /mnt/build mount point'
        sudo mkdir -p /mnt/build
        abortIfNonZero $? "creating /mnt/build"
    fi

    installLxc "${lxcFs}"

    # Try to temporarily mount the device to get an accurate FS-type reading.
    sudo mount "${device}" /mnt 1>&2 2>/dev/null
    fs=$(sudo df -T $device | tail -n1 | sed 's/[ \t]\+/ /g' | cut -d' ' -f2)
    test -z "${fs}" && echo "error: failed to determine FS type for ${device}" 1>&2 && exit 1
    sudo umount "${device}" 1>&2 2>/dev/null
    abortIfNonZero "umounting device=${device}"

    # Strip leading '/' from $zfsPool, this is actually a compatibility update for 2017.
    zfsPoolArg="$(echo "${zfsPool}" | sed 's/^\///')"

    echo "info: existing fs type on ${device} is ${fs}"
    if [ "${fs}" = "${lxcFs}" ] ; then
        echo "info: ${device} is already formatted with ${lxcFs}"
        if [ "${lxcFs}" = 'zfs' ] ; then
            setupLxdWithZfs "${zfsPoolArg}"
        fi
    else
        # Purge any pre-existing fstab /mnt entries.
        sudo sed -i '/.*[ \t]\/mnt[ \t].*/d' /etc/fstab

        echo "info: formatting ${device} with ${lxcFs}"
        if [ "${lxcFs}" = 'btrfs' ] ; then
            sudo mkfs.btrfs $device
            abortIfNonZero $? "mkfs.btrfs ${device}"

            echo "info: updating /etc/fstab to map /mnt/build to the ${lxcFs} device"
            if [ -z "$(grep "$(echo $device | sed 's:/:\\/:g')" /etc/fstab)" ] ; then
                echo "info: adding new fstab entry for ${device}"
                echo "${device} /mnt/build auto defaults 0 0" | sudo tee -a /etc/fstab >/dev/null
                abortIfNonZero $? "fstab add"
            else
                echo 'info: editing existing fstab entry'
                sudo sed -i "s-^\s*${device}\s\+.*\$-${device} /mnt/build auto defaults 0 0-" /etc/fstab
                abortIfNonZero $? "fstab edit"
            fi

            echo "info: mounting device ${device}"
            sudo mount $device
            abortIfNonZero $? "mounting ${device}"

        elif [ "${lxcFs}" = 'zfs' ] ; then
            setupLxdWithZfs "${zfsPoolArg}"

            # Unmount all ZFS volumes.
            sudo zfs umount -a
            abortIfNonZero $? "command 'sudo zfs umount -a'"

            # Export the pool.
            sudo zpool export "${zfsPoolArg}"
            abortIfNonZero $? "command 'sudo zpool export ${zfsPoolArg}'"

            # Import the zfs pool, this will mount the volumes.
            sudo zpool import "${zfsPoolArg}"
            abortIfNonZero $? "command 'sudo zpool import ${zfsPoolArg}'"

            # Enable zfs snapshot listing.
            sudo zpool set listsnapshots=on "${zfsPoolArg}"
            abortIfNonZero $? "command 'sudo zpool set listsnapshots=on ${zfsPoolArg}'"

            # Add zfsroot to lxc configuration.
            test -z "$(sudo grep '^lxc.lxcpath *=' /etc/lxc/lxc.conf 2>/dev/null)" && echo "lxc.lxcpath = /${zfsPoolArg}/lxc" | sudo tee -a /etc/lxc/lxc.conf || sudo sed -i "s/^lxc.lxcpath *=.*/lxc.lxcpath = \/${zfsPoolArg}\/lxc/g" /etc/lxc/lxc.conf
            test -z "$(sudo grep '^lxc.bdev.zfs.root *=' /etc/lxc/lxc.conf 2>/dev/null)" && echo "lxc.bdev.zfs.root = ${zfsPoolArg}/lxc" | sudo tee -a /etc/lxc/lxc.conf || sudo sed -i "s/^lxc.bdev.zfs.root *=.*/lxc.bdev.zfs.root = ${zfsPoolArg}\/lxc/g" /etc/lxc/lxc.conf
            abortIfNonZero $? 'application of lxc zfsroot setting'

            # Remove any fstab entry for the ZFS device (ZFS will auto-mount one pool).
            sudo sed -i "/.*$(echo "${device}" | sed 's/\//\\\//g').*/d" /etc/fstab

            # Chmod 777 /${zfsPoolArg}/git
            sudo chmod 777 "/${zfsPoolArg}/git"
            abortIfNonZero $? "command 'sudo chmod 777 /${zfsPoolArg}/git'"

            # # Link /var/lib/lxc to /${zfsPoolArg}/lxc, and then link /mnt/build/lxc to /var/lib/lxc.
            # test -d '/var/lib/lxc' && sudo mv /var/lib/lxc{,.bak} || :
            # test ! -h '/var/lib/lxc' && sudo ln -s "/${zfsPoolArg}/lxc" /var/lib/lxc || :
            # test ! -h '/mnt/build/lxc' && sudo ln -s "/${zfsPoolArg}/lxc" /mnt/build/lxc || :

            # # Also might as well resolve the git linkage while we're here.
            # test ! -h '/mnt/build/git' && sudo ln -s "/${zfsPoolArg}/git" /mnt/build/git || :
            # test ! -h '/git' && sudo ln -s "/${zfsPoolArg}/git" /git || :

        else
            echo "error: prepareNode() got unrecognized filesystem=${lxcFs}" 1>&2
            exit 1
        fi
    fi

    if [ -d /var/lib/lxc ] && ! [ -e /mnt/build/lxc ] ; then
        echo 'info: creating and linking /mnt/build/lxc folder'
        sudo mv /{var/lib,mnt/build}/lxc
        abortIfNonZero $? "lxc directory migration"
        sudo ln -s /mnt/build/lxc /var/lib/lxc
        abortIfNonZero $? "lxc directory symlink"
    fi

    if ! [ -d /mnt/build/lxc ] ; then
        echo 'info: attempting to create missing /mnt/build/lxc'
        sudo mkdir /mnt/build/lxc
        abortIfNonZero $? "lxc directory creation"
    fi

    if ! [ -e /var/lib/lxc ] ; then
        echo 'info: attemtping to symlink missing /var/lib/lxc to /mnt/build/lxc'
        sudo ln -s /mnt/build/lxc /var/lib/lxc
        abortIfNonZero $? "lxc directory symlink 2nd attempt"
    fi

    if ! [ -z "${swapDevice}" ] && [ -e "${swapDevice}" ] ; then
        echo "info: activating swap device or partition: ${swapDevice}"
        # Ensure the swap device target us unmounted.
        sudo umount "${swapDevice}" 1>/dev/null 2>/dev/null || :
        # Purge any pre-existing fstab entries before adding the swap device.
        sudo sed -i "/^$(echo "${swapDevice}" | sed 's/\//\\&/g')[ \t].*/d" /etc/fstab
        abortIfNonZero $? "purging pre-existing ${swapDevice} entries from /etc/fstab"
        echo "${swapDevice} none swap sw 0 0" | sudo tee -a /etc/fstab 1>/dev/null
        sudo swapoff --all
        abortIfNonZero $? "adding ${swapDevice} to /etc/fstab"
        sudo mkswap -f "${swapDevice}"
        abortIfNonZero $? "mkswap ${swapDevice}"
        sudo swapon --all
        abortIfNonZero $? "sudo swapon --all"
    fi

    # Install updated kernel if running Ubuntu 12.x series so 'lxc exec' will work.
    majorVersion=$(lsb_release --release | sed 's/^[^0-9]*\([0-9]*\)\..*$/\1/')
    if [ ${majorVersion} -eq 12 ] ; then
        echo 'info: installing 3.8 or newer kernel, a system restart will be required to complete installation'
        sudo apt-get install -y linux-generic-lts-raring-eol-upgrade
        abortIfNonZero $? 'installing linux-generic-lts-raring-eol-upgrade'
        echo 1 | sudo tee -a /tmp/SB_RESTART_REQUIRED
    fi

    echo 'info: installing automatic zpool importer to system bootscript /etc/rc.local'
    test -e /etc/rc.local || (sudo touch /etc/rc.local && sudo chmod a+x /etc/rc.local)
    test $(grep '^sudo zpool import '"${zfsPoolArg}"' -f$' /etc/rc.local | wc -l) -eq 0 && sudo sed -i 's/^exit 0$/sudo zpool import '"${zfsPoolArg}"' -f/' /etc/rc.local && \
        test $(grep '^sudo zpool import '"${zfsPoolArg}"' -f$' /etc/rc.local | wc -l) -eq 0 && echo 'sudo zpool import '"${zfsPoolArg}"' -f' | sudo tee -a /etc/rc.local
    grep "^sudo zpool import ${zfsPoolArg} -f"'$' /etc/rc.local
    abortIfNonZero $? 'adding zpool auto-mount to /etc/rc.local' 1>&2

    echo 'info: prepareNode() succeeded'
}

function prepareLoadBalancer() {
    # @param $1 ssl certificate base filename (without path).
    certFile=/tmp/$1

    version=$(lsb_release -a 2>/dev/null | grep "Release" | grep -o "[0-9\.]\+$")

    if [ "${version}" = "16.04" ] ; then
        ppa='ppa:vbernat/haproxy-1.7'
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
net.core.wmem_max = 16777216' | sudo tee /etc/sysctl.d/60-shipbuilder.conf
    abortIfNonZero $? 'writing out /etc/sysctl.d/60-shipbuilder.conf'

    sudo systemctl restart procps
    abortIfNonZero $? 'service procps restart'

    echo "info: adding ppa repository for ${version}: ${ppa}"
    sudo apt-add-repository -y "${ppa}"
    abortIfNonZero $? "adding apt repository ppa ${ppa}"

    required="haproxy ntp"
    echo "info: installing required packages: ${required}"
    sudo apt-get update
    abortIfNonZero $? "updating apt"
    sudo apt-get install -y ${required}
    abortIfNonZero $? "apt-get install ${required}"

    optional="vim-haproxy"
    echo "info: installing optional packages: ${optional}"
    sudo apt-get install -y ${optional}
    abortIfNonZero $? "apt-get install ${optional}"

    if [ -r "${certFile}" ] ; then
        if ! [ -d "/etc/haproxy/certs.d" ] ; then
            sudo mkdir /etc/haproxy/certs.d 2>/dev/null
            abortIfNonZero $? "creating /etc/haproxy/certs.d directory"
        fi

        echo "info: installing ssl certificate to /etc/haproxy/certs.d"
        sudo mv $certFile /etc/haproxy/certs.d/
        abortIfNonZero $? "moving certificate to /etc/haproxy/certs.d"

        sudo chmod 400 /etc/haproxy/certs.d/$(echo $certFile | sed "s/^.*\/\(.*\)$/\1/")
        abortIfNonZero $? "chmod 400 /etc/haproxy/certs.d/<cert-file>"

        sudo chown -R haproxy:haproxy /etc/haproxy/certs.d
        abortIfNonZero $? "chown haproxy:haproxy /etc/haproxy/certs.d"

    else
        echo "warn: no certificate file was provided, ssl support will not be available" 1>&2
    fi

    echo "info: enabling the HAProxy system service in /etc/default/haproxy"
    sudo sed -i "s/ENABLED=0/ENABLED=1/" /etc/default/haproxy
    abortIfNonZero $? "enabling haproxy service in /dev/default/haproxy"
    echo 'info: prepareLoadBalancer() succeeded'
}

function installGo() {
    if [ -z "$(command -v go)" ] ; then
        echo "info: installing go v${goVersion}"
        local downloadUrl="https://storage.googleapis.com/golang/go${goVersion}.linux-amd64.tar.gz"
        echo "info: downloading go binary distribution from url=${downloadUrl}"
        curl --silent --fail "${downloadUrl}" > "go${goVersion}.tar.gz"
        abortIfNonZero $? "downloading go-lang binary distribution"
        sudo tar -C /usr/local -xzf "go${goVersion}.tar.gz"
        abortIfNonZero $? "decompressing and installing go binary distribution to /usr/local"
        sudo tee /etc/profile.d/Z99-go.sh << EOF
export GOROOT='/usr/local/go'
export GOPATH="\${HOME}/go"
export PATH="\${PATH}:\${GOROOT}/bin:\${GOPATH}/bin"
EOF
        abortIfNonZero $? "creating /etc/profile.d/Z99-go.sh"
        source /etc/profile
        mkdir -p "${GOPATH}/bin" 2>/dev/null
        abortIfNonZero $? "creating directory ${GOPATH}/bin"
    else
        echo 'info: go already appears to be installed, not going to force it'
    fi
}

function gitLinkage() {
    echo 'info: creating and linking /mnt/build/git -> /git'
    test ! -d '/mnt/build/git' && test ! -h '/mnt/build/git' && sudo mkdir -p /mnt/build/git || :
    test -d '/mnt/build/git' && sudo chown 777 /mnt/build/git || :
    test ! -h '/git' && sudo ln -s /mnt/build/git /git || :
    echo 'info: git linkage succeeded'
}

function buildEnv() {
    echo 'info: add build servers hostname configuration'
    test ! -d /mnt/build/env && sudo rm -rf /mnt/build/env && sudo mkdir -p /mnt/build/env || :
    abortIfNonZero $? "creating directory /mnt/build/env"
    echo "${sbHost}" | sudo tee /mnt/build/env/SB_SSH_HOST
    abortIfNonZero $? "appending shipbuilder host to /mnt/build/env/SB_SSH_HOST"
    echo 'info: env configuration succeeded'
}

function rsyslogLoggingListeners() {
    echo 'info: enabling rsyslog listeners'
    echo '# UDP syslog reception.
    $ModLoad imudp
    $UDPServerAddress 0.0.0.0
    $UDPServerRun 514

    # TCP syslog reception.
    $ModLoad imtcp
    $InputTCPServerRun 10514' | sudo tee /etc/rsyslog.d/49-haproxy.conf
    echo 'info: restarting rsyslog'
    sudo systemctl restart rsyslog
    if [ -e /etc/rsyslog.d/haproxy.conf ] ; then
        echo 'info: detected existing rsyslog haproxy configuration, will disable it'
        sudo mv /etc/rsyslog.d/haproxy.conf /etc/rsyslog.d-haproxy.conf.disabled
    fi
    echo 'info: rsyslog configuration succeeded'
}

function getContainerIp() {
    local container=$1
    local allowedAttempts=60
    local i=0

    test -z "${container}" && echo 'error: getContainerIp() missing required parameter: $container' 1>&2 && exit 1 || :

    echo "info: getting container ip-address for name '${container}'"
    while [ ${i} -lt ${allowedAttempts} ] ; do
        maybeIp="$(sudo lxc list --format=json | jq -r ".[] | select(.name==\"${container}\") | .state.network.eth0.addresses[].address" | grep '[0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}')"
        # Verify that after a few seconds the ip hasn't changed.
        if [ -n "${maybeIp}" ] ; then
            sleep 1
            ip="$(sudo lxc list --format=json | jq -r ".[] | select(.name==\"${container}\") | .state.network.eth0.addresses[].address" | grep '[0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}')"
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
    local container=$1
    local skipIfExists=$2
    local lxcFs=$3

    test -z "${container}" && echo 'error: lxcInitContainer() missing required parameter: $container' 1>&2 && exit 1
    test -z "${lxcFs}" && echo 'error: lxcInitContainer() missing required parameter: $lxcFs' 1>&2 && exit 1

    lxcContainerExists "${container}"
    existsRc=$?
    if [ ${skipIfExists} -eq 1 ] && [ ${existsRc} -eq 0 ] ; then
        echo "info: lxcInitContainer() skipping container=${container} because it already exists and the skip flag was passed"
    else
        echo "info: clearing any pre-existing container=${container}"
        sudo lxc delete --force "${container}"

        echo "info: creating lxc container=${container}"
        sudo lxc launch "${lxcBaseImage}" "${container}"
        abortIfNonZero $? "sudo lxc launch ubuntu:16.04 "${container}""

        getContainerIp "${container}"

        lxcConfigContainer "${container}"
        abortIfNonZero $? "lxcInitContainer() lxcConfigContainer() failed for container=${container}"
    fi
}

function lxcConfigContainer() {
    # @param $1 container name.
    local container=$1

    test -z "${container}" && echo 'error: lxcConfigContainer() missing required parameter: $container' 1>&2 && exit 1

    echo "info: adding shipbuilder server's public-key to authorized_keys file in container=${container}"

    sudo lxc exec -T "${container}" -- bash -c 'set -e && sudo -u ubuntu mkdir -p /home/ubuntu/.ssh && chown -R ubuntu:ubuntu /home/ubuntu/.ssh && chmod 700 /home/ubuntu/.ssh'
    abortIfNonZero $? "creation of container=${container} ~/.ssh directory"

    sudo lxc exec -T "${container}" -- sudo -u ubuntu ls -lah /home/ubuntu/

    sudo lxc exec -T "${container}" -- sudo -u ubuntu tee /home/ubuntu/.ssh/authorized_keys < ~/.ssh/id_rsa.pub
    abortIfNonZero $? "creation of container=${container} ssh authorized_keys"

    sudo lxc exec -T "${container}" -- chmod 600 /home/ubuntu/.ssh/authorized_keys
    abortIfNonZero $? "chmod 600 container=${container} .ssh/authorized_keys"

    echo 'info: adding the container "ubuntu" user to the sudoers list'
    sudo lxc exec -T "${container}" -- bash -c 'set -e && echo "ubuntu ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers'
    abortIfNonZero $? "adding 'ubuntu' to container=${container} sudoers"

    echo "info: updating apt repositories in container=${container}"
    ssh -o 'StrictHostKeyChecking=no' -o 'BatchMode=yes' "ubuntu@${ip}" "sudo apt-get update"
    abortIfNonZero $? "container=${container} apt-get update"

    packages='daemontools git-core curl unzip'
    echo "info: installing packages to container=${container}: ${packages}"
    ssh -o 'StrictHostKeyChecking=no' -o 'BatchMode=yes' "ubuntu@${ip}" "sudo apt-get install -y ${packages}"
    abortIfNonZero $? "container=${container} apt-get install -y ${packages}"

    echo "info: removing $(shipbuilder containers list-purge-packages | tr $'\n' ' ') packages"
    sudo lxc exec -T "${container}" -- apt-get purge -y $(shipbuilder containers list-purge-packages | tr $'\n' ' ')
    abortIfNonZero $? "container=${container} apt-get purge -y $(shipbuilder containers list-purge-packages | tr $'\n' ' ')"

    echo "info: disabling unnecessary system services - $(shipbuilder containers list-disable-services | tr $'\n' ' ')"
    shipbuilder containers list-disable-services | sudo lxc exec -T "${container}" -- bash -c "xargs -n1 -IX /bin/bash -c 'systemctl is-enabled X 1>/dev/null && ( systemctl stop X ; systemctl disable X )'"
    abortIfNonZero $? "container=${container} disabling unnecessary system services - $(shipbuilder containers list-disable-services | tr $'\n' ' ')"

    echo "info: stopping container=${container}"
    sudo lxc stop --force "${container}"

    echo "info: configuration succeeded for container=${container}"
}

function lxcContainerExists() {
    local container=$1

    test -z "${container}" && echo 'error: lxcContainerExists() missing required parameter: $container' 1>&2 && exit 1

    # Test whether or not the container already exists.
    test -z "$(sudo lxc list --format=json | jq -r ".[] | select(.name==\"${container}\")")"
}

function lxcContainerRunning() {
    local container=$1

    test -z "${container}" && echo 'error: lxcContainerRunning() missing required parameter: $container' 1>&2 && exit 1

    # Test whether or not the container already exists and is running.
    test "$(sudo lxc list --format=json | jq -r ".[] | select(.name==\"${container}\") | .status")" = 'Running'
}

function lxcDestroyContainer() {
    local  __resultvar=$1
    local container=$2
    local lxcFs=$3

    test -z "${container}" && echo 'error: lxcDestroyContainer() missing required parameter: $container' 1>&2 && exit 1
    test -z "${lxcFs}" && echo 'error: lxcDestroyContainer() missing required parameter: $lxcFs' 1>&2 && exit 1

    local result=0

    lxcContainerExists "${container}"
    existsRc=$?
    if [ ${existsRc} -eq 0 ] ; then
        sudo lxc stop --force "${container}"
        local attempts=10
        while [ ${attempts} -gt 0 ] ; do
            sudo lxc delete --force "${container}"
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
            test "${lxcFs}" = 'zfs' && sudo zfs destroy "tank/base@${container}" || true
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
    local buildPack=$1
    local skipIfExists=$2
    local lxcFs=$3
    local container="base-${buildPack}"
    set -x

    test -z "${buildPack}" && echo 'error: lxcConfigBuildPack() missing required parameter: $buildPack' 1>&2 && exit 1 || :
    test -z "${skipIfExists}" && echo 'error: lxcConfigBuildPack() missing required parameter: $skipIfExists' 1>&2 && exit 1 || :
    test -z "${lxcFs}" && echo 'error: lxcConfigBuildPack() missing required parameter: $lxcFs' 1>&2 && exit 1 || :

    local packagesFile="${SB_REPO_PATH}/build-packs/${buildPack}/container-packages"
    test ! -r "${packagesFile}" && echo "error: lxcConfigBuildPack() missing packages file for build-pack '${buildPack}': '${packagesFile}' not found" 1>&2 && exit 1
    local packages="$(cat "${packagesFile}" 2>/dev/null | tr -d '\n')"

    local customCommandsFile="${SB_REPO_PATH}/build-packs/${buildPack}/container-custom-commands"
    test ! -r "${customCommandsFile}" && echo "error: lxcConfigBuildPack() missing custom commands file for build-pack '${buildPack}': '${customCommandsFile}' not found" 1>&2 && exit 1

    # Test if the container was left in a running state, and if so, destroy it (since failed runs can leave things partially done).
    lxcContainerRunning "${container}"
    runningRc=$?
    if [ ${alreadyRunning} -eq 0 ] ; then
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
        sudo lxc copy base "${container}"
        sudo lxc start "${container}"
        getContainerIp "${container}"

        # Install packages.
        echo "info: installing packages to ${container} container: ${packages}"
        #ssh -o 'StrictHostKeyChecking=no' -o 'BatchMode=yes' "ubuntu@${ip}" "sudo apt install -y ${packages}"
        sudo lxc exec -T "${container}" -- /bin/bash -c "sudo apt update && sudo apt -o Dpkg::Options::='--force-overwrite' install -y ${packages}"
        rc=$?
        if [ ${rc} -ne 0 ] ; then
            echo 'warning: first attempt at installing packages failed, falling back to trying one by one..'
            for package in ${packages} ; do
                #ssh -o 'StrictHostKeyChecking=no' -o 'BatchMode=yes' "ubuntu@${ip}" "sudo apt install -y ${package}"
                sudo lxc exec -T "${container}" -- /bin/bash -c "sudo apt install -o Dpkg::Options::='--force-overwrite' -y ${package}"
                abortIfNonZero $? "[${container}] container apt install -y ${package}"
            done
            #sudo lxc exec -T "${container}" -- sed -i 's/^NTPSERVERS=".*"$/NTPSERVERS=""/' /etc/default/ntpdate
            #abortIfNonZero $? "[${container}] container sed -i 's/^NTPSERVERS=\".*\"$/NTPSERVERS=\"\"/' /etc/default/ntpdate"
            ##
            # NB: NTPSERVERS override disabled because not all build-packs have ntp* installed.
            ##
        fi

        # Run custom container commands.
        if [ -n "${customCommandsFile}" ] ; then
            echo "info: running customCommandsFile: ${customCommandsFile}"
            #ssh -o 'StrictHostKeyChecking=no' -o 'BatchMode=yes' "ubuntu@${ip}" "${customCommands}"
            rsync -azve 'ssh -o "StrictHostKeyChecking=no" -o "BatchMode=yes"' "${customCommandsFile}" "ubuntu@${ip}:/tmp/custom.sh"
            abortIfNonZero ${rc} "[${container}] rsyncing customCommandsFile=${customCommandsFile} to ubuntu@${ip}:/tmp/custom.sh failed"
            sudo lxc exec -T "${container}" -- sudo /bin/bash /tmp/custom.sh
            rc=$?
            # Cleanup temp custom commands script.
            #sudo lxc exec -T "${container}" -- sudo rm -f /tmp/custom.sh
            abortIfNonZero ${rc} "[${container}] container customCommandsFile=${customCommandsFile}"
        fi

        echo "info: stopping ${container} container"
        sudo lxc stop --force "${container}"
        abortIfNonZero $? "[${container}] lxc stop --force ${container}"
        echo 'info: build-pack configuration succeeded'
    fi
}

function lxcConfigBuildPacks() {
    # @param $2 lxc filesystem to use.
    local skipIfExists=$1
    local lxcFs=$2

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
    sbHost=$1
    device=$2
    lxcFs=$3
    local zfsPool=$4
    local swapDevice=$5
    test -z "${sbHost}" && echo 'error: prepareServerPart1(): missing required parameter: shipbuilder host' 1>&2 && exit 1 || :
    test -z "${device}" && echo 'error: prepareServerPart1(): missing required parameter: device' 1>&2 && exit 1 || :
    test -z "${lxcFs}" && echo 'error: prepareServerPart1(): missing required parameter: lxcFs' 1>&2 && exit 1 || :
    test "${device}" = "${swapDevice}" && echo 'error: prepareServerPart1() device & swapDevice must be different' 1>&2 && exit 1 || :
    test "${lxcFs}" = 'zfs' && test -z "${zfsPool}" && echo 'error: prepareServerPart1() missing required zfs parameter: $zfsPool' 1>&2 && exit 1 || :

    prepareNode "${device}" "${lxcFs}" "${zfsPool}" "${swapDevice}"
    abortIfNonZero $? 'prepareNode() failed'

    installGo
    abortIfNonZero $? 'installGo() failed'

    gitLinkage
    abortIfNonZero $? 'gitLinkage() failed'

    buildEnv
    abortIfNonZero $? 'buildEnv() failed'

    rsyslogLoggingListeners
    abortIfNonZero $? 'rsyslogLoggingListeners() failed'

    echo 'info: prepareServerPart1() succeeded'
}

function prepareServerPart2() {
    # @param $2 lxc filesystem to use.
    local skipIfExists=$1
    local lxcFs=$2

    test -z "${skipIfExists}" && echo 'error: prepareServerPart2() missing required parameter: $skipIfExists' 1>&2 && exit 1 || :
    test -z "${lxcFs}" && echo 'error: prepareServerPart2() missing required parameter: $lxcFs' 1>&2 && exit 1 || :

    local container='base'

    lxcInitContainer "${container}" "${skipIfExists}" "${lxcFs}"
    abortIfNonZero $? "lxcInitContainer(container=${container}, lxcFs=${lxcFs}, skipIfExists=${skipIfExists}) failed"

    lxcConfigBase 'base' "${skipIfExists}"
    abortIfNonZero $? "lxcConfigBase('base' skipIfExists=\"${skipIfExists}\")"

    lxcConfigBuildPacks "${skipIfExists}" "${lxcFs}"
    abortIfNonZero $? "lxcConfigBuildPacks(lxcFs=${lxcFs}, skipIfExists=${skipIfExists}) failed"

    echo 'info: prepareServerPart2() succeeded'
}

function installSingleBuildPack() {
    # @param $3 lxc filesystem to use.
    local buildPack=$1
    local skipIfExists=$2
    local lxcFs=$3

    test -z "${buildPack}" && echo 'error: installSingleBuildPack() missing required parameter: $buildPack' 1>&2 && exit 1
    test -z "${skipIfExists}" && echo 'error: installSingleBuildPack() missing required parameter: $skipIfExists' 1>&2 && exit 1
    test -z "${lxcFs}" && echo 'error: installSingleBuildPack() missing required parameter: $lxcFs' 1>&2 && exit 1

    lxcConfigBuildPack "${buildPack}" "${skipIfExists}" "${lxcFs}"

    echo 'info: installSingleBuidlPack() succeeded'
}
