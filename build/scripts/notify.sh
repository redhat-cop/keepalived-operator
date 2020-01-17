## $file contains the file to be watched
## $pid contains the file with the PID to ne notigied with SIGHUP

set -o nounset
set -o errexit

notify -t ./notify-template.sh /proc/$(cat $pid)/root/$file