function verifySshAndSudoForHosts() {
    # @param $1 string. List of space-delimited SSH connection strings.
    local sshHosts="$1"
    echo "info: Verifying ssh and sudo access for $(echo "${sshHosts}" | tr ' ' '\n' | grep -v '^ *$' | wc -l | sed 's/^[ \t]*//g') hosts"
    for sshHost in $(echo "${sshHosts}"); do
        echo -n "info:     testing host ${sshHost} .. "
        result=$(ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' -o 'ConnectTimeout 15' -q $sshHost 'sudo -n echo "succeeded" 2>/dev/null')
        rc=$?
        if [ $rc -ne 0 ]; then
            echo 'failed'
            echo "error: ssh connection test failed for host: ${sshHost}" 1>&2
            exit 1
        fi
        if [ -z "${result}" ]; then
            echo 'failed'
            echo "error: sudo access test failed for host: ${sshHost}" 1>&2
            exit 1
        fi
        echo 'succeeded'
    done
}

