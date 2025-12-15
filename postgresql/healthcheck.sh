#!/bin/bash

# Check if PostgreSQL is responding
if pg_isready -U postgres > /dev/null 2>&1; then
    exit 0
else
    exit 1
fi
