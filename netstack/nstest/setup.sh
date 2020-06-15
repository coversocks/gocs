/sbin/ip link set dev tap0 up mtu 1500
/sbin/ip addr add dev tap0 local 192.168.1.200 peer 192.168.1.1
/sbin/ip route add 0.0.0.0/0 via 10.211.55.1 src 10.211.55.23
/sbin/ip route add 47.56.85.233/32 via 10.211.55.1
/sbin/ip route add 116.202.244.153/32 via 10.211.55.1
/sbin/ip route add 0.0.0.0/1 via 192.168.100.200
/sbin/ip route add 128.0.0.0/1 via 192.168.100.200
/sbin/ip route add 192.168.100.1/32 via 192.168.100.200

/sbin/ip link set dev tap0 up mtu 1500
/sbin/ip addr add dev tap0 local 192.168.1.200 peer 192.168.1.1
/sbin/ip route add 0.0.0.0/1 via 192.168.100.1
/sbin/ip route add 128.0.0.0/1 via 192.168.100.1
/sbin/ip route add 192.168.100.200/32 via 192.168.100.1

/sbin/ip route del 192.168.1.1/32
/sbin/ip route del 47.56.85.233/32
/sbin/ip route del 0.0.0.0/1
/sbin/ip route del 128.0.0.0/1
# /sbin/ip addr del dev tun0 local 192.168.1.200 peer 192.168.1.200


ip route replace default via 10.211.55.1 src 10.211.55.23



/sbin/ip tuntap add mode tun dev tun0
/sbin/ip link set dev tun0 up mtu 1500
/sbin/ip addr add dev tun0 local 192.168.100.6 peer 192.168.100.5
/sbin/ip route add 47.56.85.233/32 via 10.211.55.1
/sbin/ip route add 0.0.0.0/1 via 192.168.100.5
/sbin/ip route add 128.0.0.0/1 via 192.168.100.5
/sbin/ip route add 192.168.100.1/32 via 192.168.100.5


/sbin/ip link set dev tun0 up mtu 1500
/sbin/ip addr add dev tun0 local 192.168.100.1 peer 192.168.100.200
/sbin/ip route add 119.23.62.69/32 via 10.211.55.1
/sbin/ip route add 0.0.0.0/1 via 192.168.255.5
/sbin/ip route add 128.0.0.0/1 via 192.168.255.5
/sbin/ip route add 192.168.255.1/32 via 192.168.255.5

ip route add 10.211.55.0/24 dev eth0 src 10.211.55.23 table rt.23
ip route add default via 10.211.55.1 dev eth0 table rt.23
ip rule add from 10.211.55.23/32 table rt.23
ip rule add to 10.211.55.23/32 table rt.23