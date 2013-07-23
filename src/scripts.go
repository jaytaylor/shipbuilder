package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"text/template"
)

const (
	PRE_RECEIVE = `#!/bin/bash
while read oldrev newrev refname
do
  ` + EXE + ` pre-receive ` + "`pwd`" + ` $oldrev $newrev $refname || exit 1
done
`
	POST_RECEIVE = `#!/bin/bash
while read oldrev newrev refname
do
  ` + EXE + ` post-receive ` + "`pwd`" + ` $oldrev $newrev $refname || exit 1
done
`
)

var POSTDEPLOY = `#!/usr/bin/python -u
import os, stat, subprocess, sys, time

def getIp(name):
  with open('/var/lib/lxc/' + name + '/rootfs/app/ip') as f:
    return f.read().split('/')[0]

def main(argv):
  #print 'main argv={0}'.format(argv)
  container = argv[1] #.split(',')
  process = argv[1].split('` + DYNO_DELIMITER + `')[-3] # process is always 3 from the end.

  # Start the specified container.
  app = container.rsplit('` + DYNO_DELIMITER + `', 3)[0] # Get rid of port + version.
  port = container.split('` + DYNO_DELIMITER + `')[-1]
  print('cloning container: ' + container)
  subprocess.call([
    '/usr/bin/lxc-clone',
    '-s',
    '-B', 'btrfs',
    '-o', app,
    '-n', container
  ], stdout=sys.stdout, stderr=sys.stderr)

  # This line, if present, will prevent the container from booting.
  print('Scrubbing any "lxc.cap.drop = mac_{0}" lines from container config'.format(container))
  subprocess.call([
    'sed', '-i',
    '/lxc.cap.drop = mac_{0}/d'.format(container),
    '/var/lib/lxc/{0}/config'.format(container),
  ])

  print('creating run script for ' + app + ' with process type=' + process)
  # NB: The curly braces are kinda crazy here, to get a single '{' or '}' with python.format(), use double curly
  # braces.
  host = '''` + sshHost + `'''
  runScript = '''#!/bin/bash
ip addr show eth0 | grep 'inet.*eth0' | awk '{{print $2}}' > /app/ip
rm -rf /tmp/log
cd /app/src
echo '{port}' > ../env/PORT
while read line || [ -n "$line" ]; do
  process="${{line%%:*}}"
  command="${{line#*: }}"
  if [ "$process" == "{process}" ]; then
    envdir ../env /bin/bash -c "${{command}} 2>&1 | /app/` + BINARY + ` logger -h{host} -a{app} -p{process}"
  fi
done < Procfile'''.format(port=port, host=host.split('@')[-1], process=process, app=app)
  runScriptFileName = '/var/lib/lxc/{0}/rootfs/app/run'.format(container)
  with open(runScriptFileName, 'w') as fh:
    fh.write(runScript)
  # Chmod to be executable.
  st = os.stat(runScriptFileName)
  os.chmod(runScriptFileName, st.st_mode | stat.S_IEXEC)

  print('starting container: {0}'.format(container))
  subprocess.call([
    '/usr/bin/lxc-start',
    '--daemon',
    '-n', container,
  ], stdout=sys.stdout, stderr=sys.stderr)

  print('Waiting for container to boot and report ip-address')
  # Allow container to bootup.
  ip = None
  for _ in xrange(45):
    time.sleep(1)
    try:
      ip = getIp(container)
    except:
      continue

  if ip:
    print('- ip: ' + ip)
    subprocess.call([
      '/sbin/iptables',
      '--table', 'nat',
      '--append', 'PREROUTING',
      '--proto', 'tcp',
      '--dport', port,
      '--jump', 'DNAT',
      '--to-destination', ip + ':' + port,
    ], stdout=sys.stdout, stderr=sys.stderr)

    # Another rule so that the port will be reachable from <eth0-ip>:port
    # e.g. $ iptables --table nat --append OUTPUT --proto tcp --dport 10001 --out-interface lo --jump DNAT --to-destination 1.2.3.4:10001
    subprocess.call([
      '/sbin/iptables',
      '--table', 'nat',
      '--append', 'OUTPUT',
      '--proto', 'tcp',
      '--dport', port,
      '--out-interface', 'lo',
      '--jump', 'DNAT',
      '--to-destination', ip + ':' + port,
    ], stdout=sys.stdout, stderr=sys.stderr)

    if process == 'web':
      print('Waiting for web-server to finish starting up')
      subprocess.call([
        '/usr/bin/curl',
        '-sL',
        '-w', '"%{http_code} %{url_effective}\\n"',
        ip + ':{0}/'.format(port),
        '-o', '/dev/null',
      ], stdout=sys.stdout, stderr=sys.stderr)

  else:
    print('- error retrieving ip')
    sys.exit(1)

main(sys.argv)`

var SHUTDOWN_CONTAINER = `#!/usr/bin/python -u

import subprocess, sys

def main(argv):
  container = argv[1]

  # Stop all existing containers.
  print('stopping container: ' + container)
  subprocess.call([
    '/usr/bin/lxc-stop',
    '-n', container,
    '-k',
  ], stdout=sys.stdout, stderr=sys.stderr)
  subprocess.call([
    '/usr/bin/lxc-destroy',
    '-n', container,
  ], stdout=sys.stdout, stderr=sys.stderr)

  cont = True
  while cont:
    cont = False
    result = subprocess.check_output(['/sbin/iptables', '--table', 'nat', '--list', '--line-numbers', '--numeric'])
    for line in result.split('\n'):
      port = container.split('` + DYNO_DELIMITER + `').pop()

      if line.find('dpt:' + port) >= 0:
        print('remove ' + ' '.join(line.split()))
        subprocess.call(
          ['/sbin/iptables', '--table', 'nat', '--delete', 'PREROUTING', line.split()[0],],
          stdout=sys.stdout, stderr=sys.stderr
        )
        subprocess.call(
          ['/sbin/iptables', '--table', 'nat', '--delete', 'OUTPUT', line.split()[0],],
          stdout=sys.stdout, stderr=sys.stderr
        )
        cont = True
        break

main(sys.argv)`

var (
	UPSTART        = template.New("UPSTART")
	HAPROXY_CONFIG = template.New("HAPROXY_CONFIG")
	BUILD_PACKS    = map[string]*template.Template{}
)

func init() {
	template.Must(UPSTART.Parse(`
console none

start on (local-filesystems and net-device-up IFACE!=lo)
stop on [!12345]
#exec su ` + DEFAULT_NODE_USERNAME + ` -c "/app/run"
exec /app/run`))

	template.Must(HAPROXY_CONFIG.Parse(`
global
    maxconn 4096
    log 127.0.0.1       local1 notice

defaults
    log global
    mode http
    option tcplog
    retries 4
    option redispatch
    maxconn 32000
    contimeout 5000
    clitimeout 30000
    srvtimeout 30000
    timeout client 30000
    #option http-server-close

frontend frontend
    bind 0.0.0.0:80
    # Require SSL
    redirect scheme https if !{ ssl_fc }
    bind 0.0.0.0:443 ssl crt /etc/haproxy/certs.d
    option httplog
    option http-pretend-keepalive
    option forwardfor
    option http-server-close
{{range $app := .Applications}}
  {{range .Domains}}
    use_backend {{$app.Name}}{{if $app.Maintenance}}-maintenance{{end}} if { hdr_dom(host) -i {{.}} }
  {{end}}
{{end}}

{{range .Applications}}
backend {{.Name}}
    balance roundrobin
    reqadd X-Forwarded-Proto:\ https if { ssl_fc }
    option forwardfor
    option abortonclose
    option httpchk GET /
  {{range .Servers}}
    server {{.Host}}-{{.Port}} {{.Host}}:{{.Port}} check port {{.Port}} observe layer7 minconn 20 maxconn 40 check inter 10s rise 1 fall 3 weight 1
  {{end}}{{if .HaProxyStatsEnabled}}
    stats enable
    stats uri /haproxy
    stats auth {{.HaProxyCredentials}}
  {{end}}

backend {{.Name}}-maintenance
    acl static_file path_end .gif || path_end .jpg || path_end .jpeg || path_end .png || path_end .css
    reqirep ^GET\ (.*)                    GET\ {{.MaintenancePageBasePath}}\1     if static_file
    reqirep ^([^\ ]*)\ [^\ ]*\ (.*)       \1\ {{.MaintenancePageFullPath}}\ \2    if !static_file
    reqirep ^Host:\ .*                    Host:\ {{.MaintenancePageDomain}}
    reqadd Cache-Control:\ no-cache,\ no-store,\ must-revalidate
    reqadd Pragma:\ no-cache
    reqadd Expires:\ 0
    rspirep ^HTTP/([^0-9\.]+)\ 200\ OK    HTTP/\1\ 503\ 
    rspadd Retry-After:\ 60
    server s3 {{.MaintenancePageDomain}}:80
{{end}}`))

	// Discover all available build-packs.
	listing, err := ioutil.ReadDir("build-packs")
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
	for _, buildPack := range listing {
		if buildPack.IsDir() {
			fmt.Printf("Discovered build-pack: %v\n", buildPack.Name())
			contents, err := ioutil.ReadFile("build-packs/" + buildPack.Name() + "/pre-hook")
			if err != nil {
				fmt.Fprintf(os.Stderr, "fatal: build-pack '%v' missing pre-hook file: %v\n", buildPack.Name(), err)
				os.Exit(1)
			}
			// Map to template.
			BUILD_PACKS[buildPack.Name()] = template.Must(template.New("BUILD_" + strings.ToUpper(buildPack.Name())).Parse(string(contents)))
		}
	}

	if len(BUILD_PACKS) == 0 {
		fmt.Fprintf(os.Stderr, "fatal: no build-packs found\n")
		os.Exit(1)
	}
}
