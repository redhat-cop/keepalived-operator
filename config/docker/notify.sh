## $file contains the source file to be watched
## $dst_file contains the destination file to be created from the source file
## $reachip contains the IP to use for interface autodiscovery, or is empty if this behavior is disabled
## $pid contains the file with the PID to be notified with SIGHUP
## $create_config_only is set to true to launch the script in one-shot mode (no notification loop)

function set_up_configs {
  cp $file $dst_file

  if [ -n "$reachip" ]; then
    IFACE=$(ip route get $reachip | grep -Po '(?<=(dev )).*(?= src| proto)')
    sed -i "s/interface.*$/interface $IFACE/g" $dst_file
    echo "autodicovered local interface that can reach $reachip to be $IFACE"
  fi
}

set -o nounset
set -o errexit

if [ "$create_config_only" = "true" ]; then
  set_up_configs
  exit 0
fi

HASH=$(md5sum $(readlink -f $file))

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