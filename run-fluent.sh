#!/bin/bash
set -x
docker run -v $PWD/fluentd/:/fluentd/etc banzaicloud/fluentd fluentd -c /fluentd/etc/fluentd.conf
