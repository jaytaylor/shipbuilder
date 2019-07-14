package scripts

// HAProxySrc is the primary Shipbuilder HA Proxy universal template, in golang.
const HAProxySrc = `
global
    maxconn 32000
    # NB: Base HAProxy logging configuration is as per: http://kvz.io/blog/2010/08/11/haproxy-logging/
    #log 127.0.0.1 local1 info
    log {{ .LogServerIpAndPort }} local1 info
    chroot /var/lib/haproxy
    stats socket /run/haproxy/admin.sock mode 660 level admin
    stats timeout 30s
    user haproxy
    group haproxy
    daemon

    # Default SSL material locations
    ca-base /etc/ssl/certs
    crt-base /etc/ssl/private

    # Default ciphers to use on SSL-enabled listening sockets.
    # For more information, see ciphers(1SSL).
    ssl-default-bind-options force-tlsv12
    ssl-default-bind-ciphers ECDH+AESGCM:DH+AESGCM:ECDH+AES256:DH+AES256:ECDH+AES128:DH+AES:RSA+AESGCM:RSA+AES:!aNULL:!MD5:!DSS

    ssl-default-server-options force-tlsv12
    ssl-default-server-ciphers ECDH+AESGCM:DH+AESGCM:ECDH+AES256:DH+AES256:ECDH+AES128:DH+AES:RSA+AESGCM:RSA+AES:!aNULL:!MD5:!DSS
   
    tune.ssl.default-dh-param 2048

defaults
    log global
    mode http
    option tcplog
    retries 4
    option redispatch
    timeout connect 5000
    timeout client 30000
    timeout server 30000
    #option http-server-close

{{- with $context := . }}

frontend frontend
    bind 0.0.0.0:80
    {{- if gt (len .SSLForwardingDomains) 0 }}
    # SSL redirect.
    {{- range $domain := .SSLForwardingDomains }}
    http-request redirect scheme https code 301 if !{ ssl_fc } { hdr(host) -i {{ $context.DynHdrFlags }}-- {{ $domain }} }
    {{- end }}
    bind 0.0.0.0:443 ssl crt /etc/haproxy/certs.d force-tlsv12
    {{- end }}
    maxconn 32000
    option httplog
    option http-pretend-keepalive
    option forwardfor
    option http-server-close

    {{- range $app := .Applications }}
    {{- range $i, $domain := .Domains }}
    acl {{ $i }}_{{ $app.Name }} hdr(host) -i {{ $context.DynHdrFlags }}-- {{ $domain }}
    {{- end }}
    {{- end }}
    {{- range $app := .Applications }}
    {{- range $i, $domain := .Domains }}
    use_backend {{ $app.Name }}{{ if $app.Maintenance }}-maintenance{{ end }} if {{ $i }}_{{ $app.Name }}
    {{- end }}
    {{- end }}

    {{- if and .HaProxyStatsEnabled .HaProxyCredentials .LoadBalancers}}
    # NB: Restrict stats vhosts to load-balancers hostnames, only.
    use_backend load_balancer if { {{ range .LoadBalancers }} hdr(host) -i {{ $context.DynHdrFlags }}-- {{ . }} {{ end }} }
    {{- end }}

{{- range $app := .Applications }}


# app: {{ .Name }}
backend {{ .Name }}
    balance roundrobin
    reqadd X-Forwarded-Proto:\ https if { ssl_fc }
    option forwardfor
    option abortonclose
    option httpchk GET / HTTP/1.1\r\nHost:\ {{ .FirstDomain }}
    {{- range $app.Servers }}
    server {{ .Host }}-{{ .Port }} {{ .Host}}:{{ .Port}} check port {{ .Port}} observe layer7
    {{- end }}

backend {{ $app.Name }}-maintenance
    acl static_file path_end .gif || path_end .jpg || path_end .jpeg || path_end .png || path_end .css
    reqirep ^GET\ (.*)                    GET\ {{ $app.MaintenancePageBasePath }}\1     if static_file
    reqirep ^([^\ ]*)\ [^\ ]*\ (.*)       \1\ {{ $app.MaintenancePageFullPath }}\ \2    if !static_file
    reqirep ^Host:\ .*                    Host:\ {{ $app.MaintenancePageDomain }}
    reqadd Cache-Control:\ no-cache,\ no-store,\ must-revalidate
    reqadd Pragma:\ no-cache
    reqadd Expires:\ 0
    rspirep ^HTTP/([^0-9\.]+)\ 200\ OK    HTTP/\1\ 503\ 
    rspadd Retry-After:\ 60
    server s3 {{ $app.MaintenancePageDomain }}:80

{{- end }}

{{- end }}

{{ if and .HaProxyStatsEnabled .HaProxyCredentials .LoadBalancers }}
backend load_balancer
    stats enable
    stats uri /haproxy
    stats auth {{ .HaProxyCredentials }}
{{- end }}

{{ if and .HaProxyStatsEnabled .HaProxyCredentials .LoadBalancers }}
frontend stats
    bind *:1337 ssl crt /etc/haproxy/certs.d force-tlsv12
    stats enable
    stats uri /tsdyno
    stats auth {{ .HaProxyCredentials }}
{{- end }}
`
