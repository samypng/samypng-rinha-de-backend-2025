#!/bin/bash

echo "Starting payment processors..."
cd payment-processor
docker-compose -f docker-compose-arm64.yml up -d
cd ..

echo "Waiting for payment processors to start..."
sleep 5

echo "Starting main application..."
docker-compose up -d --remove-orphans

echo "All services started!"
echo ""
echo "🔗 Service URLs:"
echo "  - App:                http://localhost:9999"
echo "  - Payment Processor (Default): http://localhost:8001"
echo "  - Payment Processor (Fallback): http://localhost:8002"
echo "  - Redis:                   localhost:6379"
echo ""
echo "To view logs:"
echo "  docker-compose logs -f"
echo ""
echo "To stop everything:"
echo "  ./stop.sh" 