#!/bin/bash

sudo_command=$(command -v sudo)

YUM=`command -v yum 2>/dev/null`
DNF=`command -v dnf 2>/dev/null`
# if DNF is available use it
if [ -x "$DNF" ]; then
	YUM=$DNF
fi

$sudo_command $YUM install -y golang make go-bindata
go get -u ./...
