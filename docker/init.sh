#!/bin/bash
# 数据库初始化脚本
# 执行顺序: init.sql (表结构) -> testdata.sql (测试数据，可选)

set -e

echo "=========================================="
echo "  GoChat Database Initialization"
echo "=========================================="

# 执行表结构初始化
if [ -f "/docker-entrypoint-initdb.d/01-init.sql" ]; then
    echo ">>> [1/2] Creating tables..."
    mysql -u root -p"${MYSQL_ROOT_PASSWORD}" < /docker-entrypoint-initdb.d/01-init.sql
    echo "<<< Tables created successfully"
fi

# 执行测试数据（如果环境变量 ENABLE_TEST_DATA=true）
if [ "${ENABLE_TEST_DATA}" = "true" ]; then
    if [ -f "/docker-entrypoint-initdb.d/02-testdata.sql" ]; then
        echo ">>> [2/2] Inserting test data..."
        mysql -u root -p"${MYSQL_ROOT_PASSWORD}" < /docker-entrypoint-initdb.d/02-testdata.sql
        echo "<<< Test data inserted successfully"
    fi
else
    echo ">>> [2/2] Test data skipped (set ENABLE_TEST_DATA=true to enable)"
fi

echo "=========================================="
echo "  Database initialization complete!"
echo "=========================================="
