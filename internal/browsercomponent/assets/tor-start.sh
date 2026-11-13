#!/bin/sh
set -eu
if [ ! -d /config/tor-data ]; then
    cp -R /opt/tor-data /config/tor-data
fi
cd /opt/tor-browser
exec ./Browser/start-tor-browser
