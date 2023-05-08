#!/bin/sh
#umask 0600
sudo -u www-data sqlite3 /var/lib/fritzdyn/fritzdyn.sqlite3 \
	'.read create_tables.sql'
