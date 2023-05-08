#!/bin/bash
go run github.com/cespare/reflex -d none -s -g .env -- bash -c ". .env; go run github.com/cespare/reflex -d none -s -G .env -G 'fritzdyn.sqlite3*' -- go run -tags server ."
