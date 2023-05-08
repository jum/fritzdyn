.read settings.sql
BEGIN;
drop table if exists updates;
drop table if exists hosts;
drop trigger if exists hosts_update;
drop trigger if exists updates_update;

create table hosts (
	token CHAR(43) PRIMARY KEY,
	name VARCHAR(255) NOT NULL,
	ip4addr varchar(255),
	ip6addr varchar(255),
	modified DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	created DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER hosts_update AFTER UPDATE ON hosts
	FOR EACH ROW WHEN OLD.modified != DATETIME()
BEGIN
	UPDATE hosts SET modified = DATETIME() where token = NEW.token;
END;

create table updates (
	id INTEGER NOT NULL PRIMARY KEY,
	token CHAR(43) NOT NULL,
	cmd VARCHAR(255) NOT NULL,
	args VARCHAR(255) NOT NULL,
	modified DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	created DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY(token) REFERENCES hosts(token)
);

CREATE TRIGGER updates_update AFTER UPDATE ON updates
	FOR EACH ROW WHEN OLD.modified != DATETIME()
BEGIN
	UPDATE updates SET modified = DATETIME() where id = NEW.id;
END;

COMMIT;
