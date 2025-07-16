#!/bin/bash

echo "Stopping Rinha Backend 2025..."

echo "Stopping main application..."
docker-compose down

echo "Stopping payment processors..."
cd payment-processor
docker-compose -f docker-compose-arm64.yml down
cd ..

echo "All services stopped!" 