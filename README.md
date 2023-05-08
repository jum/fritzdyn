# fritzdyn

Fritzdyn is go server for the AVM FritzBox dynamic DNS update protocol. The program needs to run
on a computer with a fixed IP so it is always reachable. It can be run as a CGI program (if you
have only a few fritzboxes), or as a permantly running server. An example for caddy with the
CGI module installed:

```
		cgi /cgi-bin/fritzdyn.cgi /usr/lib/cgi-bin/fritzdyn.cgi {
			env NODE_ENV=development SQL_DRIVER=sqlite3 SQL_DSN=/var/lib/fritzdyn/fritzdyn.sqlite3?_fk=true&_journal=WAL
		}
```
