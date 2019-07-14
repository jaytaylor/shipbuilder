# TODOs

- [ ] Run `systemctl status app` on dynos to ensure service is running as part of the startup health check.

- [ ] Cleann up leftover dynos when a deploy fails.

- [ ] Fix redeploy failures

```
ubuntu@ip-x:~/maintenance$ sb redeploy -a my-app
0:01 === Redeploying my-app
redeploying
0:01 App image container already exists
0:01 $ /snap/bin/lxc stop --force my-app
0:02 Building image
0:03 $ /snap/bin/lxc start my-app
0:04 detected IP=10.181.163.83 for container=my-app
0:05 $ /bin/bash -c set -o errexit ; set -o pipefail ; set -o nounset ; chown -R root:root /git/my-app && chmod -R a+rwx /git/my-app
0:05 $ /bin/bash -c set -o errexit ; set -o pipefail ; set -o nounset ; test -n "$(/snap/bin/lxc config device list my-app | grep '^git$')"
0:05 $ /bin/bash -c set -o errexit ; set -o pipefail ; set -o nounset ; /snap/bin/lxc config device add my-app git disk source=/git/my-app path=/git
0:05 Device git added to my-app
0:07 $ /bin/bash -c set -o errexit ; set -o pipefail ; set -o nounset ; test -n "$(/snap/bin/lxc config device list my-app | grep '^git$')"
0:07 $ /bin/bash -c set -o errexit ; set -o pipefail ; set -o nounset ; /snap/bin/lxc config device remove my-app git
0:08 Device git removed from my-app
building: exit status 128 (output=fatal: reference is not a tree: d5596925da035508eb991d99473ffc98791293a1
)
```

