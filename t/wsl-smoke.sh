#!/bin/sh
# Run from the repository root: sudo unshare -n sh t/wsl-smoke.sh
set -eu
[ "$(readlink /proc/self/ns/net)" != "$(readlink /proc/1/ns/net)" ]
ip link set lo up
testroot=$(mktemp -d)
mkdir "$testroot/logs"
root=$(pwd)
cat > "$testroot/nginx.conf" <<EOF
worker_processes 1;
error_log stderr warn;
events {}
http {
  access_log off;
  init_by_lua_block {
    local file = "$root/lua/ip_blacklist.lua"
    local snapshot = "$root/examples/abuseipdb-30d/snapshot.bin"
    dofile("$root/t/cidr-check.lua")(file, snapshot)
    local m = dofile(file)
    m.init({path=snapshot, cidrs={"127.0.0.0/24"},
            default_site={enabled=true, whitelist={"127.0.0.1"}}})
    assert(m.lookup("34.51.168.107") and m.lookup("34.51.169.76"))
    assert(not m.lookup("34.51.168.108") and not m.lookup("34.51.169.77"))
    package.loaded.ip_blacklist = m
  }
  server { listen 127.0.0.1:18080;
    location / { access_by_lua_block { require("ip_blacklist").access() }
                 content_by_lua_block { ngx.exit(204) } }
  }
}
EOF
nginx=/usr/local/openresty/nginx/sbin/nginx
"$nginx" -p "$testroot/" -c nginx.conf -g 'daemon off;' &
pid=$!
trap 'kill "$pid" 2>/dev/null || :; wait "$pid" 2>/dev/null || :; rm -f "$testroot/nginx.conf"' EXIT
probe() { curl --noproxy '*' --interface "$1" -s -o /dev/null -w '%{http_code}' --max-time 1 http://127.0.0.1:18080/; }
for i in $(seq 1 50); do
    if [ "$(probe 127.0.0.1)" = 204 ]; then break; fi
    sleep 0.1
done
[ "$(probe 127.0.0.1)" = 204 ]
[ "$(probe 127.0.1.1)" = 204 ]
rc=0; code=$(probe 127.0.0.2) || rc=$?
[ "$rc" = 52 ] && [ "$code" = 000 ] || exit 1
echo "PASS: suffix boundaries, CIDR validation, allowlist and real 444 close"
