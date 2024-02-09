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

Or using a docker container:

```
name: fritzdyn
services:
  fritzdyn:
    container_name: fritzdyn
    restart: always
    image: jumager/fritzdyn:master
    volumes:
      - /run/containers:/run/containers
      - ./data:/data
    environment:
      SQL_DRIVER: sqlite
      SQL_DSN: /data/fritzdyn.sqlite3?_journal_mode=WAL&_fk=true
      NODE_ENV: production
      CF_API_KEY: CLOUDFLARE_API_TOKEN_VALUE
      PORT: /run/containers/fritzdyn.sock
    labels:
     caddy: fritzdyn.example.org
     caddy.tls: admin@example.org
     caddy.tls.dns: cloudflare {env.CF_API_KEY}
     caddy.import: norobots
     caddy.skip_log: /health
     caddy.reverse_proxy: "unix//run/containers/fritzdyn.sock"
networks:
  default:
    name: caddy
    external: true
```
