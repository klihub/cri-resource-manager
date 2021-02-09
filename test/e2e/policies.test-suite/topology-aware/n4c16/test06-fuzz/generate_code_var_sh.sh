#!/bin/bash
cd "$(dirname "$0")" || {
    echo "cannot cd to the directory of $0"
    exit 1
}
docker run -v "$(pwd):/mnt/models" fmbt:latest sh -c 'cd /mnt/models; fmbt fuzz.fmbt.conf 2>/dev/null | fmbt-log -f \$as\$al' | grep -v AAL | sed -e 's/^, /  /g' -e 's/^\([^i].*\)/echo "TESTGEN: \1"/g' -e 's/^i:\(.*\)/\1; kubectl get pods -A; vm-command "date +%T.%N"/g' > code.var.sh
