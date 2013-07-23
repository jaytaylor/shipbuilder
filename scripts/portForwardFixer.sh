#!/usr/bin/env bash

IFS_BAK=$IFS
export IFS=$'\n'

for line in $(sudo lxc-ls --fancy | grep 'RUNNING' | grep '_web_'); do
    port=$(echo "${line}" | cut -d' ' -f1 | grep -o '[0-9]\+$')
    ip=$(echo "${line}" | sed 's/ \+/ /g' | cut -d' ' -f3)
    postrouting=$(sudo iptables --table nat --list PREROUTING --numeric | sed '1,2d' | sed 's/ \+/ /g')
    rule="to:${ip}:${port}"
    echo "port=$port, line=$ip"
    ruleFound=$(echo "${postrouting}" | grep "${rule}")
    if [ -n "${ruleFound}" ]; then
        echo "found rule ${rule}"
    else
        echo "rule not found: ${rule}"
        echo 'adding it'
        sudo iptables --table nat --append PREROUTING --proto tcp --dport $port --jump DNAT --to-destination "${ip}:${port}"
        sudo iptables --table nat --append OUTPUT --proto tcp --dport $port --out-interface lo --jump DNAT --to-destination "${ip}:${port}"
    fi
done

export IFS=$IFS_BAK
unset IFS_BAK

