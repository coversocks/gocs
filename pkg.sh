#!/bin/bash
##############################
#####Setting Environments#####
echo "Setting Environments"
set -xe
export cpwd=`pwd`
export LD_LIBRARY_PATH=/usr/local/lib:/usr/lib
#### Package ####
srv_ver=v1.6.1
srv_name=coversocks
build=$cpwd/build
output=$cpwd/build/$srv_name-$srv_ver
out_dir=$srv_name-$srv_ver
srv_out=$output/$srv_name
go_path=`go env GOPATH`
go_os=`go env GOOS`
go_arch=`go env GOARCH`

##build normal
cat <<EOF > version.go
package gocs

const Version = "$srv_ver"
EOF
echo "Build $srv_name normal executor..."
go build -o $srv_out/coversocks github.com/coversocks/gocs/coversocks
cp -f coversocks-install.sh $srv_out
cp -f coversocks.service $srv_out
cp -f default-*.json docker-*.json $srv_out
cp -f csuser.json $srv_out
cp -f gfwlist.txt $srv_out
cp -f abp.js $srv_out
cp -f networksetup-*.sh $srv_out
cp -f run.sh $srv_out
cp -f Dockerfile entrypoint.sh $output

##normal package
cd $output
out_tar=$srv_name-$go_os-$go_arch-$srv_ver.tar.gz
rm -f $out_tar
tar -czvf $build/$out_tar $srv_name

##docker package
having=`docker image ls -q coversocks:$srv_ver`
if [ "$having" != "" ];then
  docker image rm -f coversocks:$srv_ver
fi
docker build -t coversocks:$srv_ver .
cd $output
docker image save coversocks:$srv_ver -o coversocks-$srv_ver.img
tar -czvf coversocks-docker-$srv_ver.tar.gz coversocks-$srv_ver.img

echo "Package $srv_name-$go_os-$go_arch-$srv_ver done..."