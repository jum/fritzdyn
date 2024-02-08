#!/bin/sh
#sudo -u www-data sqlite3 /var/lib/fritzdyn/fritzdyn.sqlite3 \
#	-cmd ".read settings.sql"
sqlite3 fritzdyn.sqlite3 \
	-cmd ".read settings.sql"
