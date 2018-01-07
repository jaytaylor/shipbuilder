ShipBuilder Server Installation
===============================

Requirements
------------
* ShipBuilder Server is compatible with Ubuntu version 12.04 and 13.04; Both have been tested and verified working (as of June 2013)
* Passwordless SSH and sudo access from your machine to all servers involved
* daemontools installed on your local machine

Mac OS-X:

    brew install daemontools

Linux:

    sudo apt-get install daemontools

* go-lang v1.8+
* AWS S3 bucket auth keys - Used to store backups of application configurations and releases on S3 for easy rollback and restoration.

Server Modules
--------------

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

Installation
------------
1. Spin up or allocate the host(s) to be used, taking note of the /dev/<DEVICE> to use for BTRFS/ZFS storage devices on the shipbuilder server and container node(s)

1.b ensure you can SSH without a password, here is an example command to add your public key to the remote servers authorized keys:
```
    ssh -i ~/.ssh/somekey.pem ubuntu@sb.example.com "echo '$(cat ~/.ssh/id_rsa.pub)' >> ~/.ssh/authorized_keys && chmod 600 .ssh/authorized_keys"
```

2. Checkout and configure ShipBuilder (via the env/ directory)
```
git clone https://github.com/Sendhub/shipbuilder.git
cd shipbuilder
cp -r env.example env

# Set the shipbuilder server host:
echo ubuntu@sb.example.com > env/SB_SSH_HOST

# Set your AWS credentials:
echo 'MY_AWS_KEY' > env/SB_AWS_KEY
echo 'MY_AWS_SECRET' > env/SB_AWS_SECRET
echo 'MY_S3_BUCKET_NAME' > env/SB_S3_BUCKET
```

This directory will need to be rsync'd to the server host under ~/go/src/github.com/jaytaylor/.

Example session:

```bash
git clone https://github.com/jaytaylor/shipbuilder.git
cd shipbuilder
cp -r env.example env

# Make edits to env/* ..

cd ..
ssh SERVERHOSTNAME mkdir -p ~/go/src/github.com/jaytaylor
rsync -azve ssh shipbuilder $SERVERHOSTNAME:~/go/src/github.com/jaytaylor/
ssh SERVERHOSTNAME bash -c 'cd ~/go/src/github.com/jaytaylor/shipbuilder && ./install/shipbuilder.sh -d /dev/sdb'
```

*or*, to just build and deploy shipbuilder (locally, so run from the server):

```bash
ssh SERVERHOSTNAME bash -c 'cd ~/go/src/github.com/jaytaylor/shipbuilder && ./install/shipbuilder.sh build-deploy'
```

3. Run Installers:

```bash
    # For shipbuilder server (make sure this device is a persistent volume as this will be the source of truth):
    ./install/shipbuilder.sh -d /dev/xvdb install

    # For nodes:
    # (note: not necessary to run this if the nodes and server are running on the same machine)
    ./install/node.sh -H ubuntu@node.example.com -d /dev/xvdb install

    # For load-balancer(s):
    ./install/load-balancer.sh -H ubuntu@lb.example.com install
```

4. Compile ShipBuilder locally:

```bash
# Install dependencies.
go get -u github.com/jteeuwen/go-bindata
make generate
go get ./...
make deps

# Build binaries.
make clean deb
sudo dpkg -i dist/shipbuilder_*.deb
sudo systemctl daemon-reload
sudo systemctl start shipbuilder
```

5. Add the load-balancer:

```bash
./shipbuilder/shipbuilder-darwin client lb:add HOST_OR_IP
```

6. Add the node(s):

```bash
./shipbuilder/shipbuilder-darwin client nodes:add HOST_OR_IP1 HOST_OR_IP2..
```

**Important:**

* Shipbuilder server must be able to reach nodes for SSH'ing on port 22.
* Nodes must be able to reach the shipbuilder server on port 8443 in order to pull down LXC container images.

Port Mappings
=============

Specific ports must be open for each module to communicate with the others.

TROUBLESHOOTING ADVICE: Be careful with your firewall rules and if/when things aren't working (like application logging, for example), always check that the port(s) in question are reachable from the load-balancer and nodes to the ShipBuilder server.

ShipBuilder Server
------------------

- `tcp/22` - Remote SSH access from SB clients (that's you!)
- `tcp/9998` - App logging
- `udp/9998` - HAProxy request logging

Container Node(s)
-----------------

- `tcp/22` - Remote SSH access from SB server
- `tcp/10000-12000` - Must be reachable from load-balancer

HAProxy Load-Balancer
---------------------

- `tcp/22` - Remote SSH access from SB server
- `tcp/80` - HTTP
- `tcp/443` - HTTPS


Health Checks
=============

All web servers must return a 200 HTTP status code response for GET requests to '/', otherwise the load-balancer will think the app is unavailable.




