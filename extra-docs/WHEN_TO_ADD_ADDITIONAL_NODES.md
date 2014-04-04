What is the criteria you would suggest for adding additional nodes to shipbuilder? Is there a known breaking point for when we should add additional hosts/nodes?

Yes, when there isn't enough available memory on most of the nodes.  If there is less then 2gb on most nodes, then you should add more.

You can use `sb nodes` to see the status of the nodes, and I sometimes also used this command to check each node manually:

    alias checkNodeMemory='for n in $(seq 1 10); do echo node number $n ; ssh "sb-node${n}a.sendhub.com" "free -m"; echo " ----------------- " ; done'

