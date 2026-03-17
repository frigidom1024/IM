# Docker 数据库初始化说明

## 文件说明

| 文件 | 说明 | 必需 |
|------|------|------|
| `init.sql` | 数据库表结构定义 | ✅ |
| `testdata.sql` | 测试数据插入脚本 | ❌ |
| `init.sh` | 自定义初始化脚本 | ❌ |

## 快速开始

### 1. 启动服务（包含测试数据）

```bash
cd docker
docker-compose up -d mysql
```

默认会自动导入：
- 表结构 (`init.sql`)
- 测试数据 (`testdata.sql`)

### 2. 仅初始化表结构（不含测试数据）

修改 `docker-compose.yaml` 中的环境变量：

```yaml
environment:
  ENABLE_TEST_DATA: "false"  # 改为 false
```

然后启动：

```bash
docker-compose up -d mysql
```

### 3. 重置数据库

```bash
# 停止并删除数据卷
docker-compose down -v

# 重新启动
docker-compose up -d mysql
```

## 手动导入 SQL

### 方式一：使用 docker exec

```bash
# 初始化表结构
docker exec -i gochat-mysql mysql -uroot -proot < init.sql

# 导入测试数据
docker exec -i gochat-mysql mysql -uroot -proot < testdata.sql
```

### 方式二：进入容器

```bash
# 进入容器
docker exec -it gochat-mysql bash

# 导入 SQL
mysql -uroot -proot < /docker-entrypoint-initdb.d/01-init.sql
mysql -uroot -proot < /docker-entrypoint-initdb.d/02-testdata.sql
```

### 方式三：使用 MySQL 客户端

```bash
mysql -h 127.0.0.1 -P 3309 -uroot -proot < init.sql
mysql -h 127.0.0.1 -P 3309 -uroot -proot < testdata.sql
```

## 测试账号

| 用户名 | 密码 | 角色 |
|--------|------|------|
| admin | 123456 | 系统管理员 |
| alice | 123456 | 开发团队群主 |
| bob | 123456 | 产品讨论组群主 |
| charlie | 123456 | 普通成员 |
| diana | 123456 | 周末约饭群群主 |
| eve | 123456 | 技术交流群主 |
| frank | 123456 | 普通成员 |
| grace | 123456 | 普通成员 |

## 连接信息

```
Host: 127.0.0.1
Port: 3309
User: root
Password: root
Database: gochat

# 或使用应用账号
User: gochat
Password: gochat
```
