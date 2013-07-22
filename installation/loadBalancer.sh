#!/usr/bin/env bash

##
# @author Jay Taylor [@jtaylor]
#
# @date 2013-07-15
#

cd "$(dirname "$0")"

while getopts “c:ht” OPTION; do
    case $OPTION in
        c)
            certFile=$OPTARG
            ;;
        h)
            echo '  -c [ssl-cert]     SSL certificate to use with HAProxy' 1>&2
            exit 1
            ;;
        t)
            DRY_RUN=1
            ;;
    esac
done

if [ -z "${certFile}" ]; then
    echo 'warn: missing ssl-cert paramter: -c [ssl-cert], no SSL support will be available' 1>&2
    #echo 'error: missing required paramter: -c [ssl-cert]' 1>&2
    #exit 1
fi

if ! [ -r "${certFile}" ]; then
    echo "warn: unable to read ssl certificate file (may not exist): ${certFile}" 1>&2
    #echo "error: unable to read ssl certificate file (may not exist): ${certFile}" 1>&2
    #exit 1
fi

#while [ -z "${lbHost}" ]; do
#    echo 'Enter the ssh login for the load-balancer:'
#    echo '    note: must be able to login and sudo from $(hostname))'
#    echo -n '# '
#    read lbHost
#done

#if [ -z "${lbHost}" ]; then
#    echo 'error: missing required paramter: -L [host]' 1>&2
#    exit 1
#fi

if [ -n "${DRY_RUN}" ]; then
    exit 0
fi


version=$(lsb_release -a 2>/dev/null | grep 'Release' | grep -o '[0-9\.]\+$')

if [ "${version}" = '12.04' ]; then
    ppa=ppa:vbernat/haproxy-1.5
elif [ "${version}" = '13.04' ]; then
    ppa=ppa:nilya/haproxy-1.5
else
    echo "error: unrecognized version of ubuntu: ${version}" 1>&2
	exit 1
fi
echo "info: Adding PPA repository for ${version}: ${ppa}"
sudo apt-add-repository -y ${ppa}

required='haproxy'
echo "info: Installing required packages: ${required}"
sudo apt-get update
sudo apt-get install -y $required

optional='vim-haproxy'
echo "info: Installing optional packages: ${optional}"
sudo apt-get install -y $optional

sudo mkdir /etc/haproxy/certs.d 2>/dev/null
if [ -r "${certFile}" ]; then
    echo 'info: Installing ssl certificate to /etc/haproxy/certs.d'
    sudo mv $certFile /etc/haproxy/certs.d/
    sudo chmod 400 /etc/haproxy/certs.d/$(echo $certFile | sed 's/^.*\/\(.*\)$/\1/')
    sudo chown -R haproxy:haproxy /etc/haproxy/certs.d
else
    echo 'warn: No ssl certificate file was provided, ssl support will NOT be available' 1>&2
fi

echo 'info: Enabling the HAProxy system service in /etc/default/haproxy'
sudo sed -i 's/ENABLED=0/ENABLED=1/' /etc/default/haproxy

exit 0

