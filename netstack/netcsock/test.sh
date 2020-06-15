#!/bin/bash
for i in {1..1000000}
do
   curl http://www.baidu.com
   sleep 1
   # nslookup http://www.baidu.com
done