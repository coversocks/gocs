#!/bin/bash
ip route add 0.0.0.0/1 via 172.18.0.1
ip route add 128.0.0.0/1 via 172.18.0.1
ip route add 172.18.0.2/32 via 172.18.0.1
ip route add 10.211.55.1/32 via 172.18.0.1

# ip route add 216.58.199.4 via 172.18.0.1
