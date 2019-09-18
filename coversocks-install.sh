#!/bin/bash

installServer(){
  if [ ! -d /home/cs ];then
    useradd cs
  fi
  mkdir -p /home/cs/coversocks
  cp -rf * /home/cs/coversocks/
  chown -R cs:cs /home/cs
  if [ ! -f /etc/systemd/system/coversocks.service ];then
    cp -f coversocks.service /etc/systemd/system/
  fi
  mkdir -p /home/cs/conf/
  if [ ! -f /home/cs/conf/coversocks.json ];then
    cp -f default-server.json /home/cs/conf/coversocks.json
  fi
  if [ ! -f /home/cs/conf/csuser.json ];then
    cp -f csuser.json /home/cs/conf/csuser.json
    chown -R cs:cs /home/cs/conf/csuser.json
  fi
  if [ ! -f /home/cs/conf/cscert.crt ];then
    /home/cs/coversocks/cert.sh /home/cs/conf/
  fi
  systemctl enable coversocks.service
}

case "$1" in
  -i)
    yum install openssl -y
    installServer
    ;;
  *)
    echo "Usage: ./coversocks-install.sh -i"
    ;;
esac