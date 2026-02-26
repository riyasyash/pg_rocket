#!/bin/bash
set -e

echo "Starting integration tests..."

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Start PostgreSQL container
echo "Starting PostgreSQL container..."
cd test/integration
docker-compose up -d
echo "Waiting for PostgreSQL to be ready..."
sleep 5

# Export DSN
export PGROCKET_SOURCE="postgres://testuser:testpass@localhost:5433/testdb"

cd ../..

# Test 1: Version command
echo -e "\n${GREEN}Test 1: Version command${NC}"
./pg_rocket version

# Test 2: Inspect command
echo -e "\n${GREEN}Test 2: Inspect command${NC}"
./pg_rocket inspect

# Test 3: Pull with parents only
echo -e "\n${GREEN}Test 3: Pull with parents only${NC}"
./pg_rocket pull --query "SELECT * FROM tasks WHERE id = 1" --parents --out test/fixtures/test1_parents.sql --verbose

# Test 4: Pull with children only
echo -e "\n${GREEN}Test 4: Pull with children (comments)${NC}"
./pg_rocket pull --query "SELECT * FROM tasks WHERE id = 2" --children comments --out test/fixtures/test2_children.sql --verbose

# Test 5: Full traversal
echo -e "\n${GREEN}Test 5: Full traversal from project${NC}"
./pg_rocket pull --query "SELECT * FROM projects WHERE id = 1" --out test/fixtures/test3_full.sql --verbose

# Test 6: JSON output
echo -e "\n${GREEN}Test 6: JSON output${NC}"
./pg_rocket pull --query "SELECT * FROM users WHERE id = 2" --parents --json --out test/fixtures/test4_json.json --verbose

# Test 7: Dry run
echo -e "\n${GREEN}Test 7: Dry run${NC}"
./pg_rocket pull --query "SELECT * FROM organizations WHERE id = 1" --dry-run

# Test 8: Row limit test
echo -e "\n${GREEN}Test 8: Row limit (should work with small dataset)${NC}"
./pg_rocket pull --query "SELECT * FROM tasks" --max-rows 20 --out test/fixtures/test5_limit.sql --verbose

# Cleanup
echo -e "\n${GREEN}Cleaning up...${NC}"
cd test/integration
docker-compose down

echo -e "\n${GREEN}All tests passed!${NC}"
