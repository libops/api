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
BUILD=false
BULK=false

while [[ $# -gt 0 ]]; do
  case $1 in
    --clean)
      CLEAN=true
      shift
      ;;
    --build)
      BUILD=true
      shift
      ;;
    --bulk)
      BULK=true
      shift
      ;;
    --help)
      echo "Usage: $0 [OPTIONS]"
      echo ""
      echo "Options:"
      echo "  --clean    Clean up test environment before and after running"
      echo "  --build    Rebuild images before running tests"
      echo "  --bulk     Load bulk seed data (200+ orgs for scale testing)"
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

# Check if bulk seed file exists when --bulk is used
if [ "$BULK" = true ]; then
  if [ ! -f "testdata/rbac_bulk_seed.sql" ]; then
    echo -e "${RED}Error: rbac_bulk_seed.sql not found!${NC}"
    echo "Run 'make generate-bulk-seed' first to generate the bulk seed data."
    exit 1
  fi
  echo -e "${YELLOW}Using bulk seed data (200+ organizations)${NC}"
  COMPOSE_FILES="-f docker-compose.yml -f docker-compose.bulk.yml"
else
  echo -e "${YELLOW}Using core seed data (3 organizations)${NC}"
  COMPOSE_FILES="-f docker-compose.yml"
fi

# Build images if requested
if [ "$BUILD" = true ]; then
  echo -e "${YELLOW}Building images...${NC}"
  docker compose $COMPOSE_FILES build
fi

# Run the tests
echo -e "${YELLOW}Starting integration tests...${NC}"
echo ""

# 1. Start backend services
echo "Starting backend services..."
docker compose $COMPOSE_FILES up -d vault mariadb

echo "Running initialization..."
if ! docker compose $COMPOSE_FILES up vault-init; then
    echo -e "${RED}✗ Initialization failed!${NC}"
    docker compose $COMPOSE_FILES down -v
    exit 1
fi

docker compose $COMPOSE_FILES up api traefik -d

docker compose logs mariadb | grep ERROR -B 7 && exit 1
echo "Running integration tests..."
if docker compose $COMPOSE_FILES up --abort-on-container-exit --exit-code-from test-runner test-runner; then
  echo ""
  echo -e "${GREEN}✓ Integration tests passed!${NC}"
  echo ""

  # Run dashboard E2E tests
  echo -e "${YELLOW}Starting dashboard E2E tests...${NC}"
  echo ""

  if docker compose $COMPOSE_FILES up --abort-on-container-exit --exit-code-from dash-tests dash-tests; then
    echo ""
    echo -e "${GREEN}✓ Dashboard E2E tests passed!${NC}"
    EXIT_CODE=0
  else
    echo ""
    echo -e "${RED}✗ Dashboard E2E tests failed!${NC}"
    EXIT_CODE=1
  fi
else
  echo ""
  echo -e "${RED}✗ Integration tests failed!${NC}"
  echo -e "${YELLOW}Skipping dashboard E2E tests due to integration test failure${NC}"
  EXIT_CODE=1
fi

exit $EXIT_CODE
