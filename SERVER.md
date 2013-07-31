ShipBuilder Server Installation
===============================

Requirements
------------
* ShipBuilder is only compatible with Ububntu version 12.04 and 13.04; Both been tested and verified working (June 2013)
* Passwordless SSH access from your machine to all servers involved
* Passwordless `sudo` access for all servers involved
* daemontools installed on your local machine
* go-lang v1.1 installed on your local machine
* AWS S3 auth keys


Overview of System Preparation
------------------------------
    1. Spin up or allocate the host(s) to be used, taking note of the /dev/<DEVICE> to use for BTRFS devices on the SB server and container nodes.
    2. Run the install script - please just let me know if you run into any issues here, this is the only place where I want to make big changes
    3. Checkout and configure ShipBuilder (via the env/ directory)
    4. Compile ShipBuilder locally (./build.sh -f)
    5. Deploy ShipBuilder (./deploy.sh -f)
    6. Add the load-balancer: ./dist/shipbuilder lb:add HOST_OR_IP
    7. Add the node(s): ./dist/shipbuilder nodes:add HOST_OR_IP1 HOST_OR_IP2.. HOST_OR_IPn
    8. Start creating apps


Service Modules
---------------

ShipBuilder is composed of 3 distinct pieces:

* ShipBuilder Server
* Container Node(s) (hosts which run the actual app containers)
* HAProxy Load-Balancer

System Layout and Topology
--------------------------

ShipBuilder can be built out with any layout you want.

Examples

Each module running on separate hosts (3+ machines):

- one machine for ShipBuilder Server
- one or more machines configured as Container Nodes
- one machine as the Load-Balancer

All modules running on a single host (1 machine):

- single machine configured with SB Sever, added as a Node and Load-Balancer


System Installation
-------------------
Once you have decided on a layout, ensure you can SSH and use `sudo` without a password on all relevant machines.

First determine which devices you want to format with BTRFS and use for /mnt/build:

0. Test/dry-run with `-t` flag:

    ```
    ./installation/install.sh -t -S [user@shipbuilder.host] -s [btrfs-device] -N [user@node1.host,user@node2.host,user@nodeN.host,..] -n [node-btrfs-device] -L [user@lb.host] -c [ssl-cert]
    ```

1. Run Environment Installer:

    ```
    ./installation/install.sh -S [user@shipbuilder.host] -s [sb-server-btrfs-device] -N [user@node1.host,user@node2.host,user@nodeN.host,..] -n [node-btrfs-device] -L [user@lb.host] -c [ssl-cert]
    ```

    Note: If you are installing everything on 1 machine, still pass all parameters, e.g.:

    ```
    ./installation/install.sh -c [ssl-cert] -L [user@host] -S [user@host] -s [sb-server-btrfs-device] -N [user@host] -n [node-btrfs-device]
    ```

2. Congratulations! The hardest part should be over.  Next, create and configure your desired settings in the `env` folder:

    ```
    cp -r env.example env
    echo 'user@host' > env/SB_SSH_HOST
    echo 'your_aws_key' > env/SB_AWS_KEY
    echo 'your_aws_secret' > env/SB_AWS_SECRET
    echo 'your_s3_bucket' > env/SB_S3_BUCKET

    # Enable HAProxy stats:
    echo '1' > SB_HAPROXY_STATS

    # Set credentials to view HAProxy stats:
    echo 'admin:password' > SB_HAPROXY_CREDENTIALS
    ```

3. Build ShipBuilder Client:

    `./build.sh`

4. Deploy to ShipBuilder Server:

    `envdir env go run deploy.go`

5. Add your load-balancer(s):

    `./dist/shipbuilder lb:add`


Port Mappings
=============

Specific ports must be open for each module.

ShipBuilder Server
------------------

- `tcp/22` - Remote SSH access
- `udp/514` - For app logging
- `udp/10514` - For app logging

Container Node(s)
-----------------

- `tcp/22` - Remote SSH access

The load-balancer must also be able to access ports 10000+ (1 port for each app instance) on all Container Nodes.

HAProxy Load-Balancer
---------------------

- `tcp/22` - Remote SSH access
- `tcp/80` - HTTP
- `tcp/443` - HTTPS


Health Checks
=============

All web servers must return a 200 HTTP status code response for GET requests to '/', otherwise the load-balancer will think the app is unavailable.


Misc
====

temporary `env` config overrides are possible, just prefix the variable=value before invoking the client:

    $ SB_SSH_HOST=sb-staging.sendhub.com ./dist/shipbuilder config -aadmin
    info: environmental override detected for SB_SSH_HOST: sb-staging.sendhub.com
    ..

Amaazon AWS
===========
If you are running this on AWS, here are some additional recommendations: /sendhub/shipbuilder/AWS_NOTES.md

