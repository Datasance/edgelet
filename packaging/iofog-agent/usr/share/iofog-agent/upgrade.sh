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

	# Find current version
	iofogpackage=$(apt-cache policy iofog-agent iofog-agent-dev 2>/dev/null | grep -A1 ^iofog | awk '$2 ~ /^[0-9]/ {print a}{a=$0}' | sed -e 's/iofog-agent\(.*\):/\1/' || echo "")
	if [ -z "$iofogpackage" ]; then
		# Try RPM
		iofogpackage=$(rpm -qa | grep iofog-agent | head -1 | sed 's/iofog-agent\(.*\)/\1/' || echo "")
	fi
	iofogversion=$(dpkg -l iofog-agent$iofogpackage 2>/dev/null | grep iofog-agent | awk '{print $3}' || rpm -q iofog-agent$iofogpackage 2>/dev/null | sed 's/iofog-agent.*-\(.*\)/\1/' || echo "unknown")

	# Copy config
	ORIGINAL="/etc/iofog-agent/config.yaml"
	BACKUP="/var/backups/iofog-agent/config.yaml"
	mkdir -p /var/backups/iofog-agent
	if [ -f "$ORIGINAL" ]; then
		cp "$ORIGINAL" "$BACKUP"
	fi

	# Stop agent
	systemctl stop iofog-agent || true

	# Create backup for rollback
	cd /var/backups/iofog-agent
	tar -cvzf config_backup$iofogpackage.tar.gz -P /etc/iofog-agent 2>/dev/null || true
	tar -cvzf log_backup_upgrade$iofogversion.tar.gz -P /var/log/iofog-agent 2>/dev/null || true
	printf 'ver: %s %s' $iofogversion $iofogpackage > prev_version_data

	# remove current configs (but keep backups)
	# Don't remove everything, just prepare for upgrade

	# Perform upgrade
	lsb_dist=$( get_distribution )
	lsb_dist="$(echo "$lsb_dist" | tr '[:upper:]' '[:lower:]')"
	case "$lsb_dist" in

	    ubuntu|debian|raspbian)
	        apt-get update
	        apt-get install --only-upgrade iofog-agent$iofogpackage -y
	    ;;

	    centos|rhel|ol|sles|fedora)
	        yum check-update || true
	        yum update iofog-agent$iofogpackage -y
	    ;;

	esac

	# Restore config and start agent
	cd /var/backups/iofog-agent
	if [ -f config_backup$iofogpackage.tar.gz ]; then
		tar -xzf config_backup$iofogpackage.tar.gz
		if [ -d etc/iofog-agent ]; then
			mv etc/iofog-agent/* /etc/iofog-agent/ 2>/dev/null || true
		fi
		echo 'config restored'
	fi

	if [ -f "$BACKUP" ]; then
		cp "$BACKUP" "$ORIGINAL"
	fi
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

} > /var/log/iofog-agent-upgrade.log 2>&1
