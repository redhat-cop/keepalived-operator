## $file contains the file to be watched
## $pid contains the file with the PID to ne notigied with SIGHUP
## $template contains the template to execute
## $verb

function set_up_configs {
  cp $file $dst_file
  if [ -n "$IFACE" ]; then
    sed -i "s/interface.*$/interface $IFACE/g" $dst_file
    echo "autodicovered local interface that can reach $reachip to be $IFACE"
  fi
}

set -o nounset
set -o errexit

HASH=$(md5sum $(readlink -f $file))

IFACE=""
if [ -n "$reachip" ]; then
  IFACE=$(ip route get $reachip | awk "/$reachip/{ print \$3 }")
fi

if [ "$setup" = "true" ]; then
  set_up_configs
  exit 0
fi

while true; do
   NEW_HASH=$(md5sum $(readlink -f $file))
   if [ "$HASH" != "$NEW_HASH" ]; then
     HASH="$NEW_HASH"
     echo "[$(date +%s)] Trigger refresh"
     set_up_configs
     kill -SIGHUP $(cat $pid); 
     echo "sent kill signal SIGHUP to $(cat $pid) with outcome $?"
   fi
   sleep 5
done