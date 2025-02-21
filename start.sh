#!/bin/bash
go tool reflex -d none -s -g .env -- bash -c ". .env; go tool reflex -d none -s -G .env -G 'fritzdyn.sqlite3*' -- go run -tags server ."
