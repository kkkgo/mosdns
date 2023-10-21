#!/bin/sh
export cf_ip="114.114.114.114 2400:3200:baba::1"
mosdns start -d /src -c test.yaml &
ls -lah  /usr/bin/mosdns
sleep 3
curl http://127.0.0.1:7889
nslookup test1.dns 127.0.0.1
nslookup test2.dns 127.0.0.1
nslookup test3.dns 127.0.0.1
nslookup test4.dns 127.0.0.1
nslookup test5.dns 127.0.0.1
nslookup test6.dns 127.0.0.1
nslookup test7.dns 127.0.0.1
nslookup test8.dns 127.0.0.1
nslookup test9.dns 127.0.0.1
nslookup test0.dns 127.0.0.1
nslookup ip_rewrite.dns 127.0.0.1