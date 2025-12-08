#!/bin/bash

echo "================================"
echo "DBRTS Explorer Test"
echo "================================"
echo ""
echo "Testing MongoDB local connection..."
echo ""

./bin/dbrts explore --config configs/mongo-local.yaml

