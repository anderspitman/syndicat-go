#!/bin/bash

curl -s -H "Accept: application/activity+json" $@ | jq
