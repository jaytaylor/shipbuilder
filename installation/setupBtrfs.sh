#!/usr/bin/env bash

##
# @author Jay Taylor [@jtaylor]
#
# @date 2013-07-15
#

cd "$(dirname "$0")"

while getopts “d:ht” OPTION; do
    case $OPTION in
        h)
            echo '  -d [device]       Device to format using BTRFS and use for /mnt/build mount' 1>&2
            exit 1
            ;;
        d)
            DEVICE=$OPTARG
            ;;
        t)
            DRY_RUN=1
            ;;
    esac
done

#echo 'info: Promt the user for the desired device'
#while [ -z "${DEVICE}" ]; do
#    echo 'Select device or partition to format using BTRFS (will be mounted in /mnt/build):'
#    declare -a devices=($(find /dev/ -regex '.*\/\([hms\|]xv\)d.*'))
#    i=0
#    while [ $i -lt ${#devices[@]} ]; do
#        dev=${devices[$i]}
#        echo "    ${i}. ${dev} ($(($(sudo blockdev --getsize64 $dev) / (1024**3)))GB)"
#        i=$(($i+1))
#    done
#    echo -n '# '
#    read selection
#
#    if [ -n "${devices[$selection]}" ]; then
#        DEVICE=${devices[$selection]}
#        break
#    else
#        echo -e '\nERROR: Invalid selection [press ENTER to continue]'
#        read
#    fi
#done

if [ -z "${DEVICE}" ]; then
    echo 'error: missing required parameter: -d [device]' 1>&2
    exit 1
fi

if ! [ -e "${DEVICE}" ]; then
    echo 'error: Exiting because an invalid device was somehow selected' 1>&2
    echo "error: unrecognized device '${DEVICE}'" 1>&2
    exit 1
fi

if [ -n "${DRY_RUN}" ]; then
    exit 0
fi

source libfns.sh
installLxc


echo "info: Unmount /mnt and ${DEVICE} to be safe"
sudo umount /mnt 1>&2 2>/dev/null
sudo umount $DEVICE 1>&2 2>/dev/null

# Try to temporarily mount the device to get an accurate FS-type reading.
sudo mount $DEVICE /mnt 1>&2 2>/dev/null
fs=$(sudo df -T $DEVICE | tail -n1 | sed 's/[ \t]\+/ /g' | cut -d' ' -f2)
test -z "${fs}" && echo "error: failed to determine FS type for ${DEVICE}" 1>&2 && exit 1
sudo umount $DEVICE 1>&2 2>/dev/null

echo "info: Existing FS type on ${DEVICE} is ${fs}"
if [ "${fs}" = "btrfs" ]; then
    echo "info: ${DEVICE} is already formatted with BTRFS"
else
    echo "info: Formatting ${DEVICE} with BTRFS"
    sudo mkfs.btrfs $DEVICE
    rc=$?
    test $rc -ne 0 && echo "error: mkfs.btrfs exited with non-zero status: ${rc}" 1>&2 && exit $rc
fi

if ! [ -d /mnt/build ]; then
    echo 'info: Creating /mnt/build mount point'
    sudo mkdir /mnt/build
else
    echo 'info: Mount point /mnt/build already exists'
fi

echo 'info: Updating /etc/fstab to map /mnt/build to the BTRFS device'
if [ -z "$(grep "$(echo $DEVICE | sed 's:/:\\/:g')" /etc/fstab)" ]; then
    echo 'info: Adding new fstab entry'
    echo "${DEVICE} /mnt/build auto defaults 0 0" | sudo tee -a /etc/fstab >/dev/null
else
    echo 'info: Editing existing fstab entry'
    sudo sed -i "s-^\s*${DEVICE}\s\+.*\$-${DEVICE} /mnt/build auto defaults 0 0-" /etc/fstab
fi

echo "info: Mounting device ${DEVICE}"
sudo mount $DEVICE
rc=$?
test $rc -ne 0 && echo "error: mounting ${DEVICE} exited with non-zero status: ${rc}" 1>&2 && exit $rc

if [ -d /var/lib/lxc ] && ! [ -e /mnt/build/lxc ]; then
    echo 'info: Create and link /mnt/build/lxc folder'
    sudo mv /{var/lib,mnt/build}/lxc
    sudo ln -s /mnt/build/lxc /var/lib/lxc
fi

if ! [ -d /mnt/build/lxc ]; then
    echo 'info: Creating missing /mnt/build/lxc'
    sudo mkdir /mnt/build/lxc
fi

if ! [ -e /var/lib/lxc ]; then
    echo 'info: Linking missing /var/lib/lxc to /mnt/build/lxc'
    sudo ln -s /mnt/build/lxc /var/lib/lxc
fi

exit 0

