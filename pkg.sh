#!/bin/bash
##############################
#####Setting Environments#####
echo "Setting Environments"
set -e
export cpwd=`pwd`
export LD_LIBRARY_PATH=/usr/local/lib:/usr/lib
export PATH=$PATH:$GOPATH/bin:$HOME/bin:$GOROOT/bin
output=$cpwd/build


#### Package ####
srv_name=coversocks
srv_ver=1.4.0
srv_out=$output/$srv_name
rm -rf $srv_out
mkdir -p $srv_out
##build normal
echo "Build $srv_name normal executor..."
go build -o $srv_out/csocks github.com/coversocks/gocs/csocks
cp -f coversocks-install.sh $srv_out
cp -f cert.sh $srv_out
cp -f coversocks.service $srv_out
cp -f default-*.json $srv_out
cp -f csuser.json $srv_out
cp -f gfwlist.txt $srv_out
cp -f abp.js $srv_out
cp -f networksetup-*.sh $srv_out
cp -f run.sh $srv_out
if [ "$UDPGW_DIR" != "" ];then
    cp -f udpgw.service $srv_out
    cd $UDPGW_DIR
    mkdir -p build
    cd build
    cmake ..
    make badvpn-udpgw
    cp -f udpgw/badvpn-udpgw $srv_out/udpgw
fi

###
cd $output
rm -f $srv_name-$srv_ver-`uname`.zip
zip -r $srv_name-$srv_ver-`uname`.zip $srv_name
cd ../
echo "Package $srv_name done..."
