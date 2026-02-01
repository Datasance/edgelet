#!/bin/bash

get_distribution() {
	lsb_dist=""
	# Every system that we officially support has /etc/os-release
	if [ -r /etc/os-release ]; then
		lsb_dist="$(. /etc/os-release && echo "$ID")"
	fi
	# Returning an empty string here should be alright since the
	# case statements don't act unless you provide an actual value
	echo "$lsb_dist"
}

{
	timeout=${2:-60}

	cd /var/backups/iofog-agent

	if [ ! -f prev_version_data ]; then
		echo "No rollback data found. Cannot perform rollback."
		exit 1
	fi

	# Stop agent
	systemctl stop iofog-agent || true

	# Perform rollback
	lsb_dist=$( get_distribution )
	lsb_dist="$(echo "$lsb_dist" | tr '[:upper:]' '[:lower:]')"
	iofogpackage=$(grep ver prev_version_data | awk '{print $3}')
	iofogversion=$(grep ver prev_version_data | awk '{print $2}')
	case "$lsb_dist" in

	    ubuntu|debian|raspbian)
	        apt-get purge --auto-remove iofog-agent$iofogpackage -y
	        apt-get install iofog-agent$iofogpackage=$iofogversion -y
	    ;;

	    centos|rhel|ol|sles|fedora)
	        yum remove iofog-agent$iofogpackage -y
	        yum install iofog-agent$iofogpackage-$iofogversion -y
	    ;;

	esac

	# Save logs
	tar -cvzf log_backup_rollback$iofogversion.tar.gz -P /var/log/iofog-agent 2>/dev/null || true
	# Overwrite config based on previous data
	if [ -f config_backup$iofogpackage.tar.gz ]; then
		rm -rf /etc/iofog-agent/
		tar -xvzf config_backup$iofogpackage.tar.gz -P -C /
		rm -rf /var/backups/iofog-agent/prev_version_data
		rm -rf /var/backups/iofog-agent/config_backup$iofogpackage.tar.gz
	fi

	# Start agent
	starttimestamp=$(date +%s)
	systemctl start iofog-agent || true
	sleep 1

	# Wait for agent
	while [ "$(iofog-agent status 2>/dev/null | grep -i running || echo "")" != "RUNNING" ]; do
		sleep 1
		currenttimestamp=$(date +%s)
		currentdeltatime=$(( $currenttimestamp - $starttimestamp ))
		if [ $currentdeltatime -gt $timeout ]; then
			break
		fi
	done

} > /var/log/iofog-agent-rollback.log 2>&1
