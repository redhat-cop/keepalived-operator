## $file contains the file to be watched
## $pid contains the file with the PID to ne notigied with SIGHUP
## $template contains the template to execute
## $verb

set -o nounset
set -o errexit

HASH=$(md5sum $(readlink -f $file))

while true; do
   NEW_HASH=$(md5sum $(readlink -f $file))
   if [ "$HASH" != "$NEW_HASH" ]; then
     HASH="$NEW_HASH"
     echo "[$(date +%s)] Trigger refresh"
     kill -SIGHUP $(cat $pid); 
     echo "sent kill signal SIGHUP to $(cat $pid) with outcome $?"
   fi
   sleep 5
done