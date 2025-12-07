#!/bin/bash

set -e

echo "LibOps Integration Test Runner"
echo "==============================="
echo ""

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Parse command line arguments
CLEAN=false
LOGS=false
BUILD=false

while [[ $# -gt 0 ]]; do
  case $1 in
    --clean)
      CLEAN=true
      shift
      ;;
    --logs)
      LOGS=true
      shift
      ;;
    --build)
      BUILD=true
      shift
      ;;
    --help)
      echo "Usage: $0 [OPTIONS]"
      echo ""
      echo "Options:"
      echo "  --clean    Clean up test environment before and after running"
      echo "  --logs     Show logs after tests complete"
      echo "  --build    Rebuild images before running tests"
      echo "  --help     Show this help message"
      echo ""
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      echo "Use --help for usage information"
      exit 1
      ;;
  esac
done

# Clean up if requested
if [ "$CLEAN" = true ]; then
  echo -e "${YELLOW}Cleaning up existing test environment...${NC}"
  docker compose down -v 2>/dev/null || true
  echo ""
fi

# Build images if requested
if [ "$BUILD" = true ]; then
  echo -e "${YELLOW}Building images...${NC}"
  docker compose build
fi

# Run the tests
echo -e "${YELLOW}Starting integration tests...${NC}"
echo ""

# 1. Start backend services
echo "Starting backend services..."
docker compose up -d vault mariadb

# 2. Run initialization
echo "Running initialization..."
if ! docker compose up vault-init; then
    echo -e "${RED}✗ Initialization failed!${NC}"
    docker compose down -v
    exit 1
fi

# 3. Run tests
echo "Running tests..."
if docker compose up --abort-on-container-exit --exit-code-from test-runner api test-runner; then
  echo ""
  echo -e "${GREEN}✓ Integration tests passed!${NC}"
  EXIT_CODE=0
else
  echo ""
  echo -e "${RED}✗ Integration tests failed!${NC}"
  EXIT_CODE=1
fi

# Show logs if requested
if [ "$LOGS" = true ]; then
  echo ""
  echo -e "${YELLOW}Test Runner Logs:${NC}"
  echo "================="
  docker compose logs test-runner
fi

exit $EXIT_CODE
