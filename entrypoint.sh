#!/bin/sh
set -xe

cd /srv/coversocks/
if [ "$MODE" == "server" ];then
    if [ ! -f /srv/conf/server.json ];then
        cp -f /srv/coversocks/docker-server.json /srv/conf/server.json
    fi
    /srv/coversocks/coversocks -s -f /srv/conf/server.json
else
    if [ ! -f /srv/conf/client.json ];then
        cp -f /srv/coversocks/docker-client.json /srv/conf/client.json
    fi
    /srv/coversocks/coversocks -c -f /srv/conf/client.json
fi

echo "coversocks is done"