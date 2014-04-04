Eventually it would be ideal to be able to bounce a node and have it automatically come back up with:
    - ZFS pool automatically imported
	- Previously running LXC containers automatically start back up
	- iptables rules automatically restored

	
TODOS:

- Need to add `sudo zpool import tank -f` to /etc/rc.local on all sb nodes

