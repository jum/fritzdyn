#!/bin/sh
# Generate a token from the openssl random number generator
# output is base64url encoded.
openssl rand -base64 32 | tr '/+' '_-' | tr -d '='
