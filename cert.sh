#!/bin/bash
openssl req -x509 -nodes -days 365 -newkey rsa:8192 -keyout $1"cscert.key" -out $1"cscert.crt" -subj "/C=TG/ST=TG/L=TG/O=Dark Socket/OU=Dark Socket/CN=xxxx"