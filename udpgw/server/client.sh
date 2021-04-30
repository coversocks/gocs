#!/bin/bash
ip tuntap add dev tun0 mode tun
ifconfig tun0 up
ifconfig tun0 10.0.0.1 netmask 255.255.255.0
echo 'nameserver 114.114.114.114' > /etc/resolv.conf
/srv/badvpn/build/tun2socks --tundev tun0 --netif-ipaddr 10.0.0.2 --netif-netmask 255.255.255.0 --socks-server-addr 10.211.55.2:7100 --udpgw-remote-server-addr 127.0.0.1:10